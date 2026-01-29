package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kcpq/client"
)

func TestSoakServerDropsSessions(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var clients []*client.Client
	for i := 0; i < 12; i++ {
		c, err := client.ConnectWithContext(ctx, s.Addr().String(), key)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		c.EnableAutoReconnect(20 * time.Millisecond)
		clients = append(clients, c)
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	var subOK atomic.Int64
	for i := 0; i < 6; i++ {
		c := clients[i]
		_, err := c.SubscribeWithContext(ctx, "drop.>", func(*client.Message) {
			subOK.Add(1)
		})
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}

	var pubOK atomic.Int64
	var wg sync.WaitGroup
	for i := 6; i < 12; i++ {
		c := clients[i]
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
					if err := c.PublishWithContext(ctx, "drop.x", []byte("x")); err == nil {
						pubOK.Add(1)
					}
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.RLock()
				var any *Session
				for sess := range s.sessions {
					any = sess
					break
				}
				s.mu.RUnlock()
				if any != nil {
					any.Close()
				}
			}
		}
	}()

	wg.Wait()

	if pubOK.Load() == 0 {
		t.Fatalf("no successful publishes")
	}
	if subOK.Load() == 0 {
		t.Fatalf("no subscriber callbacks observed")
	}
}
