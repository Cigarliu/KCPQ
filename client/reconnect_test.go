package client

import (
	"context"
	"testing"
	"time"

	"github.com/kcpq/server"
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

func TestAutoReconnectWorksAfterMultipleDisconnects(t *testing.T) {
	key := testAES256Key()
	s, err := server.NewServer("127.0.0.1:0", key)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Close()

	addr := s.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, err := ConnectWithContext(ctx, addr, key)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	c.EnableAutoReconnect(20 * time.Millisecond)

	c.mu.RLock()
	firstConn := c.conn
	c.mu.RUnlock()

	if firstConn == nil {
		t.Fatalf("expected non-nil conn")
	}

	firstConn.Close()

	waitUntil(t, 2*time.Second, func() bool {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.conn != nil && c.conn != firstConn
	})

	subCtx1, cancelSub1 := context.WithTimeout(ctx, 2*time.Second)
	defer cancelSub1()
	if _, err := c.SubscribeWithContext(subCtx1, "test.reconnect.1", func(*Message) {}); err != nil {
		t.Fatalf("subscribe after first reconnect: %v", err)
	}

	c.mu.RLock()
	secondConn := c.conn
	c.mu.RUnlock()

	if secondConn == nil {
		t.Fatalf("expected non-nil conn after first reconnect")
	}

	secondConn.Close()

	waitUntil(t, 2*time.Second, func() bool {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.conn != nil && c.conn != secondConn
	})

	subCtx2, cancelSub2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancelSub2()
	if _, err := c.SubscribeWithContext(subCtx2, "test.reconnect.2", func(*Message) {}); err != nil {
		t.Fatalf("subscribe after second reconnect: %v", err)
	}
}
