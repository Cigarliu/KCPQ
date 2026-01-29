package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kcpq/server"
)

func TestSoakPublishWhileConnDrops(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	c, err := ConnectWithContext(ctx, s.Addr().String(), key)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	c.EnableAutoReconnect(20 * time.Millisecond)

	var ok atomic.Int64
	var fails atomic.Int64

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				subject := fmt.Sprintf("soak.%d", time.Now().UnixNano()%32)
				if err := c.PublishWithContext(ctx, subject, []byte("x")); err != nil {
					fails.Add(1)
				} else {
					ok.Add(1)
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(35 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.mu.RLock()
				conn := c.conn
				c.mu.RUnlock()
				if conn != nil {
					_ = conn.Close()
				}
			}
		}
	}()

	wg.Wait()

	if ok.Load() == 0 {
		t.Fatalf("no successful publishes; fails=%d", fails.Load())
	}
	if fails.Load() > ok.Load()*10 {
		t.Fatalf("too many failures: ok=%d fails=%d", ok.Load(), fails.Load())
	}
}
