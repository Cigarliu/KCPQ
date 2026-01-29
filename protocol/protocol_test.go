package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseFromReaderRejectsOversizeBody(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(MaxBodyLen+1))
	r := bytes.NewReader(hdr[:])
	if _, err := ParseFromReader(r); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseWithRemainingRejectsOversizeBody(t *testing.T) {
	packet := make([]byte, 4)
	binary.BigEndian.PutUint32(packet[:], uint32(MaxBodyLen+1))
	if _, _, err := ParseWithRemaining(packet); err == nil {
		t.Fatalf("expected error")
	}
}

func FuzzParseWithRemaining(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{0, 0, 0, 1, 1, 0, 0})
	f.Add([]byte{0, 0, 0, 3, CmdPing, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = ParseWithRemaining(b)
	})
}

func FuzzParseFromReader(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{0, 0, 0, 3, CmdPing, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = ParseFromReader(bytes.NewReader(b))
	})
}
