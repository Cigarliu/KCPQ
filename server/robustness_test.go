package server

import (
	"context"
	"testing"
	"time"

	"github.com/kcpq/client"
)

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func TestSessionCloseSafe(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = 1
	}

	srv, err := NewServer("127.0.0.1:0", key)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	sess := &Session{
		server:  srv,
		outChan: make(chan []byte, 1),
		done:    make(chan struct{}),
	}
	srv.sessions[sess] = true

	if err := sess.SendEncoded([]byte{1}); err != nil {
		t.Fatalf("send before close: %v", err)
	}

	sess.Close()
	sess.Close()

	if err := sess.SendEncoded([]byte{2}); err == nil {
		t.Fatalf("expected error after close")
	}
	if _, ok := srv.sessions[sess]; ok {
		t.Fatalf("expected session removed from server")
	}
}

func TestServerSessionRemovedAfterClientClose(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = 1
	}

	s, err := NewServer("127.0.0.1:0", key)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Close()

	addr := s.Addr().String()

	c, err := client.ConnectWithContext(context.Background(), addr, key)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_ = c.Close()

	waitUntil(t, 2*time.Second, func() bool {
		return s.GetStats().Connections == 0
	})
}
