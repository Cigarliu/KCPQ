package server

import (
	"bytes"
	"sync"
)

// bufferPool 用于复用 buffer，减少内存分配
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// getBuffer 从 pool 获取 buffer
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer 将 buffer 放回 pool
func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() < 1<<20 { // 如果 buffer 大于 1MB，不放回 pool
		bufferPool.Put(buf)
	}
}

// bytesPool 用于复用字节切片
var bytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 4096) // 预分配 4KB
		return &b
	},
}

// getBytes 从 pool 获取字节切片
func getBytes() *[]byte {
	p := bytesPool.Get().(*[]byte)
	*p = (*p)[:0] // 重置长度
	return p
}

// putBytes 将字节切片放回 pool
func putBytes(b *[]byte) {
	if cap(*b) < 1<<16 { // 如果切片大于 64KB，不放回 pool
		bytesPool.Put(b)
	}
}
