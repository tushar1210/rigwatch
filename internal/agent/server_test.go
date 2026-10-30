package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/tushar1210/rigwatch/internal/alert"
	"github.com/tushar1210/rigwatch/internal/collect"
	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

func testSnap() metrics.Snapshot {
	return metrics.Snapshot{Time: time.Now(), Host: metrics.Host{Hostname: "forge", OS: "linux"}}
}

func TestSnapshotAndToken(t *testing.T) {
	h := newHub()
	h.publish(testSnap())
	s := &Server{Token: "secret", Interval: time.Second, Collector: &collect.Fake{Snap: testSnap()}}
	ts := httptest.NewServer(s.handler(h))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/snapshot", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var got metrics.Snapshot
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Host.Hostname != "forge" {
		t.Fatalf("host = %+v", got.Host)
	}
}

func TestSSE(t *testing.T) {
	h := newHub()
	h.publish(testSnap())
	s := &Server{Collector: &collect.Fake{Snap: testSnap()}}
	ts := httptest.NewServer(s.handler(h))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	buf := make([]byte, 4096)
	n, _ := res.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "event: snapshot") || !strings.Contains(body, "forge") {
		t.Fatalf("sse = %q", body)
	}
}

func TestWebSocket(t *testing.T) {
	h := newHub()
	h.publish(testSnap())
	s := &Server{Token: "t", Collector: &collect.Fake{Snap: testSnap()}}
	ts := httptest.NewServer(s.handler(h))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer t"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got metrics.Snapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Host.Hostname != "forge" {
		t.Fatalf("host = %+v", got.Host)
	}
}

func TestStdio(t *testing.T) {
	s := &Server{Interval: 20 * time.Millisecond, Collector: &collect.Fake{Snap: testSnap()}}
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	errc := make(chan error, 1)
	go func() {
		errc <- s.Run(ctx, "", true, pw)
	}()
	var got metrics.Snapshot
	done := make(chan error, 1)
	go func() {
		done <- ReadSnapshots(pr, func(snap metrics.Snapshot) error {
			got = snap
			cancel()
			return io.EOF
		})
	}()
	select {
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timeout")
	case err := <-done:
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
	}
	if got.Host.Hostname != "forge" {
		t.Fatalf("snap = %+v", got.Host)
	}
}

func TestStdioAlertEvent(t *testing.T) {
	cfg := config.Default()
	cfg.Alerts.Baseline.Enabled = false
	hot := metrics.Snapshot{
		Host:  metrics.Host{Hostname: "forge"},
		Temps: []metrics.Temp{{Name: "Tctl", Celsius: 90, Kind: "package"}},
	}
	s := &Server{
		Interval:  20 * time.Millisecond,
		Collector: &collect.Fake{Snap: hot},
		Alerts:    alert.NewEngine(cfg),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	errc := make(chan error, 1)
	go func() {
		errc <- s.Run(ctx, "", true, pw)
	}()
	type seen struct {
		snap  bool
		alert bool
	}
	got := make(chan seen, 1)
	go func() {
		sc := bufio.NewScanner(pr)
		var have seen
		for sc.Scan() {
			line := sc.Text()
			if strings.Contains(line, `"type":"snapshot"`) && strings.Contains(line, "forge") {
				have.snap = true
			}
			if strings.Contains(line, `"type":"alert"`) && strings.Contains(line, "cpu temp") {
				have.alert = true
			}
			if have.snap && have.alert {
				got <- have
				cancel()
				return
			}
		}
		got <- have
	}()
	select {
	case <-time.After(2 * time.Second):
		cancel()
		_ = pr.Close()
		t.Fatal("timeout")
	case have := <-got:
		cancel()
		_ = pr.Close()
		if !have.snap || !have.alert {
			t.Fatalf("stdio frames = %+v", have)
		}
	}
}

func TestStdioShutdown(t *testing.T) {
	halted := make(chan struct{})
	s := &Server{Halt: func() error {
		close(halted)
		return nil
	}, Collector: &collect.Fake{Snap: testSnap()}}
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = s.serveStdioReader(ctx, newHub(), io.Discard, pr)
	}()
	if _, err := io.WriteString(pw, "{\"type\":\"shutdown\"}\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-halted:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown was not requested")
	}
	cancel()
	_ = pw.Close()
}

func TestStdioShutdownRefusal(t *testing.T) {
	s := &Server{Halt: func() error { return errors.New("must be root") }, Collector: &collect.Fake{Snap: testSnap()}}
	pr, pw := io.Pipe()
	var out lockedBuf
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = s.serveStdioReader(ctx, newHub(), &out, pr)
	}()
	if _, err := io.WriteString(pw, "{\"type\":\"shutdown\"}\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(out.String(), "must be root") {
		if time.Now().After(deadline) {
			t.Fatalf("refusal = %q", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	_ = pw.Close()
}

func TestWebSocketShutdown(t *testing.T) {
	halted := make(chan struct{}, 1)
	h := newHub()
	h.publish(testSnap())
	s := &Server{Halt: func() error {
		halted <- struct{}{}
		return errors.New("must be root")
	}, Collector: &collect.Fake{Snap: testSnap()}}
	ts := httptest.NewServer(s.handler(h))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"shutdown"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-halted:
	case <-ctx.Done():
		t.Fatal("shutdown was not requested")
	}
	var got []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(string(got), "must be root") {
		readCtx, cancelRead := context.WithTimeout(ctx, 300*time.Millisecond)
		_, data, err := conn.Read(readCtx)
		cancelRead()
		if err != nil {
			continue
		}
		got = append(got, data...)
	}
	if !strings.Contains(string(got), "must be root") {
		t.Fatalf("reply = %s", got)
	}
}

func TestReaderStreamShutdownRefusal(t *testing.T) {
	pr, pw := io.Pipe()
	st := newReaderStream(pr, nil)
	defer func() { _ = st.Close() }()
	go func() {
		_, _ = io.WriteString(pw, "{\"type\":\"shutdown\",\"ok\":false,\"error\":\"must be root\"}\n")
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := st.Recv(ctx)
	var cmd *CommandError
	if !errors.As(err, &cmd) || cmd.Error() != "must be root" {
		t.Fatalf("err = %v", err)
	}
	_ = pw.Close()
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
