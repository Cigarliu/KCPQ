package client

import (
	"fmt"
	"sync"

	"github.com/xtaci/kcp-go"
)

func newAES256BlockCrypt(key []byte) (kcp.BlockCrypt, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("aes-256 key must be 32 bytes, got %d", len(key))
	}
	bc, err := kcp.NewAESBlockCrypt(key)
	if err != nil {
		return nil, err
	}
	return &lockedBlockCrypt{bc: bc}, nil
}

type lockedBlockCrypt struct {
	mu sync.Mutex
	bc kcp.BlockCrypt
}

func (l *lockedBlockCrypt) Encrypt(dst, src []byte) {
	l.mu.Lock()
	l.bc.Encrypt(dst, src)
	l.mu.Unlock()
}

func (l *lockedBlockCrypt) Decrypt(dst, src []byte) {
	l.mu.Lock()
	l.bc.Decrypt(dst, src)
	l.mu.Unlock()
}
