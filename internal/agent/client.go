package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/coder/websocket"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Stream yields snapshots until Close or ctx cancellation.
type Stream interface {
	Recv(ctx context.Context) (metrics.Snapshot, error)
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
		snap, err := decodeLine(data)
		if errors.Is(err, errSkipFrame) {
			continue
		}
		return snap, err
	}
}

func (s *wsStream) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

type readerStream struct {
	ch     chan metrics.Snapshot
	errc   chan error
	closer io.Closer
	cancel context.CancelFunc
}

func newReaderStream(r io.Reader, closer io.Closer) *readerStream {
	ctx, cancel := context.WithCancel(context.Background())
	st := &readerStream{
		ch:     make(chan metrics.Snapshot, 4),
		errc:   make(chan error, 1),
		closer: closer,
		cancel: cancel,
	}
	go func() {
		err := ReadSnapshots(r, func(snap metrics.Snapshot) error {
			select {
			case <-ctx.Done():
				return io.EOF
			case st.ch <- snap:
				return nil
			}
		})
		st.errc <- err
		close(st.ch)
	}()
	return st
}

func (s *readerStream) Recv(ctx context.Context) (metrics.Snapshot, error) {
	select {
	case <-ctx.Done():
		return metrics.Snapshot{}, ctx.Err()
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
	cmd *exec.Cmd
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
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	rs := newReaderStream(stdout, stdout)
	return &sshStream{readerStream: rs, cmd: cmd}, nil
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
