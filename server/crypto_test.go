package server

import "testing"

func TestNewServerRejectsInvalidAES256Key(t *testing.T) {
	if _, err := NewServer("127.0.0.1:0", nil); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := NewServer("127.0.0.1:0", make([]byte, 31)); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := NewServer("127.0.0.1:0", make([]byte, 33)); err == nil {
		t.Fatalf("expected error")
	}
}
