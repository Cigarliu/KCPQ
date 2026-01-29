package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kcpq/server"
)

func TestReconnectRestoresSubscriptionWithAck(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = 1
	}

	s, err := server.NewServer("127.0.0.1:0", key)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	subClient, err := ConnectWithContext(ctx, s.Addr().String(), key)
	if err != nil {
		t.Fatalf("connect sub: %v", err)
	}
	defer subClient.Close()
	subClient.EnableAutoReconnect(20 * time.Millisecond)
	subClient.SetMaxReconnectInterval(200 * time.Millisecond)

	var received atomic.Int64
	_, err = subClient.SubscribeWithContext(ctx, "recon.>", func(*Message) {
		received.Add(1)
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pubClient, err := ConnectWithContext(ctx, s.Addr().String(), key)
	if err != nil {
		t.Fatalf("connect pub: %v", err)
	}
	defer pubClient.Close()

	if err := pubClient.PublishWithContext(ctx, "recon.x", []byte("1")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && received.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatalf("expected to receive initial message")
	}

	subClient.mu.RLock()
	conn := subClient.conn
	subClient.mu.RUnlock()
	if conn == nil {
		t.Fatalf("expected conn")
	}
	_ = conn.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && subClient.GetStats().ReconnectCount == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if subClient.GetStats().ReconnectCount == 0 {
		t.Fatalf("expected reconnect")
	}

	before := received.Load()
	if err := pubClient.PublishWithContext(ctx, "recon.y", []byte("2")); err != nil {
		t.Fatalf("publish after reconnect: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && received.Load() == before {
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == before {
		t.Fatalf("expected to receive after reconnect")
	}
}
