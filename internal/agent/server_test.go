package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
