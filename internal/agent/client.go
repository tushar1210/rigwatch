package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/coder/websocket"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// CommandError is a refusal from the agent, such as a shutdown the OS rejected.
// The stream stays open.
type CommandError struct {
	Msg string
}

func (e *CommandError) Error() string {
	if e == nil || e.Msg == "" {
		return "command failed"
	}
	return e.Msg
}

// Stream yields snapshots until Close or ctx cancellation.
// Shutdown asks the remote agent to power the host off.
type Stream interface {
	Recv(ctx context.Context) (metrics.Snapshot, error)
	Shutdown(ctx context.Context) error
	Close() error
}

// Open connects to a parsed target.
func Open(ctx context.Context, target Target, token, remoteBin string) (Stream, error) {
	switch target.Kind {
	case "ssh":
		return OpenSSH(ctx, target, remoteBin)
	case "ws":
		return OpenWebSocket(ctx, target.WSURL, token)
	default:
		return nil, fmt.Errorf("unknown target kind %q", target.Kind)
	}
}

type wsStream struct {
	conn *websocket.Conn
}

// OpenWebSocket dials a rigwatch agent WebSocket.
func OpenWebSocket(ctx context.Context, url, token string) (Stream, error) {
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, err
	}
	return &wsStream{conn: conn}, nil
}

func (s *wsStream) Recv(ctx context.Context) (metrics.Snapshot, error) {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return metrics.Snapshot{}, err
		}
		if cmd, ok := commandError(data); ok {
			return metrics.Snapshot{}, cmd
		}
		snap, err := decodeLine(data)
		if errors.Is(err, errSkipFrame) {
			continue
		}
		return snap, err
	}
}

func (s *wsStream) Shutdown(ctx context.Context) error {
	var buf bytes.Buffer
	if err := WriteShutdown(&buf); err != nil {
		return err
	}
	return s.conn.Write(ctx, websocket.MessageText, bytes.TrimSpace(buf.Bytes()))
}

func (s *wsStream) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

type readerStream struct {
	ch     chan metrics.Snapshot
	errc   chan error
	notify chan error
	closer io.Closer
	cancel context.CancelFunc
}

func newReaderStream(r io.Reader, closer io.Closer) *readerStream {
	ctx, cancel := context.WithCancel(context.Background())
	st := &readerStream{
		ch:     make(chan metrics.Snapshot, 4),
		errc:   make(chan error, 1),
		notify: make(chan error, 4),
		closer: closer,
		cancel: cancel,
	}
	go readStream(ctx, r, st)
	return st
}

func readStream(ctx context.Context, r io.Reader, st *readerStream) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if cmd, ok := commandError(line); ok {
			select {
			case <-ctx.Done():
				st.errc <- ctx.Err()
				close(st.ch)
				return
			case st.notify <- cmd:
			}
			continue
		}
		snap, err := decodeLine(line)
		if errors.Is(err, errSkipFrame) {
			continue
		}
		if err != nil {
			st.errc <- err
			close(st.ch)
			return
		}
		select {
		case <-ctx.Done():
			st.errc <- ctx.Err()
			close(st.ch)
			return
		case st.ch <- snap:
		}
	}
	err := sc.Err()
	if err == nil {
		err = io.EOF
	}
	st.errc <- err
	close(st.ch)
}

func commandError(line []byte) (*CommandError, bool) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, false
	}
	if env.Type != "shutdown" || env.OK == nil || *env.OK {
		return nil, false
	}
	msg := env.Error
	if msg == "" {
		msg = "shutdown failed"
	}
	return &CommandError{Msg: msg}, true
}

func (s *readerStream) Recv(ctx context.Context) (metrics.Snapshot, error) {
	select {
	case <-ctx.Done():
		return metrics.Snapshot{}, ctx.Err()
	case err := <-s.notify:
		if err == nil {
			err = &CommandError{Msg: "command failed"}
		}
		return metrics.Snapshot{}, err
	case snap, ok := <-s.ch:
		if ok {
			return snap, nil
		}
		err := <-s.errc
		if err == nil || err == io.EOF {
			return metrics.Snapshot{}, io.EOF
		}
		return metrics.Snapshot{}, err
	}
}

func (s *readerStream) Close() error {
	s.cancel()
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

type sshStream struct {
	*readerStream
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

// OpenSSH runs `rigwatch agent --stdio` on the remote host.
func OpenSSH(ctx context.Context, target Target, remoteBin string) (Stream, error) {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, fmt.Errorf("ssh client not found")
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8"}
	if target.SSHPort != "" {
		args = append(args, "-p", target.SSHPort)
	}
	args = append(args, target.SSHDest, RemoteCommand(remoteBin))
	cmd := exec.CommandContext(ctx, "ssh", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	rs := newReaderStream(stdout, stdout)
	return &sshStream{readerStream: rs, cmd: cmd, stdin: stdin}, nil
}

func (s *sshStream) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.stdin == nil {
		return errors.New("not connected")
	}
	var buf bytes.Buffer
	if err := WriteShutdown(&buf); err != nil {
		return err
	}
	_, err := s.stdin.Write(buf.Bytes())
	return err
}

func (s *sshStream) Close() error {
	err := s.readerStream.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
	return err
}

// DecodeOne is exported for tests that already have a JSON payload.
func DecodeOne(data []byte) (metrics.Snapshot, error) {
	return decodeLine(data)
}
