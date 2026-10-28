package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/tushar1210/rigwatch/internal/collect"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Server publishes samples over HTTP, WebSocket, SSE, or stdio.
type Server struct {
	Token     string
	Collector collect.Collector
	Interval  time.Duration
}

type hub struct {
	mu   sync.Mutex
	last metrics.Snapshot
	has  bool
	subs map[chan metrics.Snapshot]struct{}
}

func newHub() *hub {
	return &hub{subs: map[chan metrics.Snapshot]struct{}{}}
}

func (h *hub) publish(s metrics.Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.last = s
	h.has = true
	for ch := range h.subs {
		select {
		case ch <- s:
		default:
		}
	}
}

func (h *hub) latest() (metrics.Snapshot, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last, h.has
}

func (h *hub) subscribe() (<-chan metrics.Snapshot, func()) {
	ch := make(chan metrics.Snapshot, 1)
	h.mu.Lock()
	if h.has {
		ch <- h.last
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

// Run samples until ctx is cancelled. stdio writes NDJSON to w and does not listen.
func (s *Server) Run(ctx context.Context, addr string, stdio bool, w io.Writer) error {
	if s.Collector == nil {
		return fmt.Errorf("no collector")
	}
	if s.Interval <= 0 {
		s.Interval = time.Second
	}
	h := newHub()
	go s.loop(ctx, h)
	if stdio {
		return s.serveStdio(ctx, h, w)
	}
	return s.listen(ctx, addr, h)
}

func (s *Server) loop(ctx context.Context, h *hub) {
	tick := time.NewTicker(s.Interval)
	defer tick.Stop()
	for {
		snap, err := s.Collector.Sample(ctx)
		if err == nil {
			h.publish(snap)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) serveStdio(ctx context.Context, h *hub, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("stdio writer is nil")
	}
	ch, cancel := h.subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case snap := <-ch:
			if err := WriteSnapshot(w, snap); err != nil {
				return err
			}
		}
	}
}

func (s *Server) listen(ctx context.Context, addr string, h *hub) error {
	if addr == "" {
		addr = ":7777"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.handler(h),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
		return nil
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) handler(h *hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		snap, ok := h.latest()
		if !ok {
			http.Error(w, "no sample yet", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
	})
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.serveSSE(w, r, h)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.serveWS(w, r, h)
	})
	return mux
}

func (s *Server) authorized(r *http.Request) bool {
	if s.Token == "" {
		return true
	}
	if r.Header.Get("X-Rigwatch-Token") == s.Token {
		return true
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.Token {
		return true
	}
	return r.URL.Query().Get("token") == s.Token
}

func (s *Server) serveSSE(w http.ResponseWriter, r *http.Request, h *hub) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, cancel := h.subscribe()
	defer cancel()
	for {
		select {
		case <-r.Context().Done():
			return
		case snap := <-ch:
			b, err := json.Marshal(snap)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request, h *hub) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	ctx := conn.CloseRead(context.Background())
	ch, cancel := h.subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-ch:
			b, err := json.Marshal(snap)
			if err != nil {
				return
			}
			writeCtx, cancelWrite := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Write(writeCtx, websocket.MessageText, b)
			cancelWrite()
			if err != nil {
				return
			}
		}
	}
}
