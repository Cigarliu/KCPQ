package client

import (
	"testing"

	"github.com/kcpq/server"
)

func testAES256Key() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = 1
	}
	return key
}

func startTestServer(t *testing.T) (*server.Server, []byte) {
	t.Helper()
	key := testAES256Key()
	s, err := server.NewServer("127.0.0.1:0", key)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, key
}
