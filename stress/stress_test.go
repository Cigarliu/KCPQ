package stress

import (
	"context"
	"crypto/rand"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kcpq/client"
	"github.com/kcpq/server"
)

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

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

func TestSoakPublishSubscribeWithReconnect(t *testing.T) {
	startG := runtime.NumGoroutine()

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

	addr := s.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var received atomic.Int64
	var published atomic.Int64
	var subErr atomic.Int64
	var pubErr atomic.Int64

	const (
		subscribers = 8
		publishers  = 8
	)

	var subs []*client.Client
	for i := 0; i < subscribers; i++ {
		c, err := client.ConnectWithContext(ctx, addr, key)
		if err != nil {
			t.Fatalf("connect sub: %v", err)
		}
		c.EnableAutoReconnect(25 * time.Millisecond)
		_, err = c.SubscribeWithContext(ctx, "stress.>", func(*client.Message) {
			received.Add(1)
		})
		if err != nil {
			subErr.Add(1)
		}
		subs = append(subs, c)
	}

	var pubs []*client.Client
	for i := 0; i < publishers; i++ {
		c, err := client.ConnectWithContext(ctx, addr, key)
		if err != nil {
			t.Fatalf("connect pub: %v", err)
		}
		c.EnableAutoReconnect(25 * time.Millisecond)
		pubs = append(pubs, c)
	}

	var wg sync.WaitGroup
	wg.Add(publishers)
	for i := 0; i < publishers; i++ {
		pub := pubs[i]
		go func(id int) {
			defer wg.Done()
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					size := 256 + (id * 17 % 512)
					payload := randomBytes(size)
					subject := fmt.Sprintf("stress.%d.%d", id, time.Now().UnixNano()%16)
					if err := pub.PublishWithContext(ctx, subject, payload); err != nil {
						pubErr.Add(1)
						continue
					}
					published.Add(1)
				}
			}
		}(i)
	}

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				i++
				if i%2 == 0 && len(pubs) > 0 {
					pubs[i%len(pubs)].Close()
				}
				if i%3 == 0 && len(subs) > 0 {
					subs[i%len(subs)].Close()
				}
			}
		}
	}()

	wg.Wait()

	if subErr.Load() != 0 {
		t.Fatalf("subscribe errors: %d", subErr.Load())
	}
	waitUntil(t, 1*time.Second, func() bool {
		return published.Load() > 100
	})
	waitUntil(t, 1*time.Second, func() bool {
		return received.Load() > 10
	})

	if pubErr.Load() > published.Load()/2 {
		t.Fatalf("too many publish errors: pubErr=%d published=%d", pubErr.Load(), published.Load())
	}

	for _, c := range pubs {
		_ = c.Close()
	}
	for _, c := range subs {
		_ = c.Close()
	}

	waitUntil(t, 2*time.Second, func() bool {
		return s.GetStats().Connections == 0
	})

	_ = s.Close()

	endG := runtime.NumGoroutine()
	if endG > startG+200 {
		t.Fatalf("goroutine leak suspected: start=%d end=%d", startG, endG)
	}
}
