package client

import (
	"context"
	"testing"
	"time"

	"github.com/kcpq/server"
)

func TestConnectWithContextRejectsInvalidAES256Key(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if _, err := ConnectWithContext(ctx, "127.0.0.1:1", nil); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := ConnectWithContext(ctx, "127.0.0.1:1", make([]byte, 31)); err == nil {
		t.Fatalf("expected error")
	}
}

func TestMismatchedAES256KeyCannotSubscribe(t *testing.T) {
	key1 := testAES256Key()
	key2 := make([]byte, 32)
	for i := range key2 {
		key2[i] = 2
	}

	s, err := server.NewServer("127.0.0.1:0", key1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Close()

	cli, err := ConnectWithContext(context.Background(), s.Addr().String(), key2)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cli.Close()

	subCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := cli.SubscribeWithContext(subCtx, "enc.mismatch", func(*Message) {}); err == nil {
		t.Fatalf("expected subscribe error")
	}
}
