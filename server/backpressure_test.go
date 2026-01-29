package server

import (
	"testing"
	"time"

	"github.com/kcpq/protocol"
)

func TestSessionBackpressureCloses(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = 1
	}

	srv, err := NewServer("127.0.0.1:0", key)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv.SetSessionBackpressure(1024, 1)
	srv.SetSessionWriteTimeout(10 * time.Millisecond)

	sess := NewSession(nil, srv)
	if err := sess.SendEncoded(protocol.NewMessageCmd(protocol.CmdMsg, "a", make([]byte, 900)).Encode()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sess.SendEncoded(protocol.NewMessageCmd(protocol.CmdMsg, "a", make([]byte, 900)).Encode()); err == nil {
		t.Fatalf("expected backpressure error")
	}
}
