package client

import (
	"sync"
	"time"
)

// ringBuffer 环形缓冲区（用于延迟统计，避免内存泄漏）
// 解决原 slice 滑动窗口导致的内存泄漏问题
type ringBuffer struct {
	data  []time.Duration
	size  int
	head  int
	tail  int
	count int
	mu    sync.Mutex
}

// newRingBuffer 创建环形缓冲区
func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		data: make([]time.Duration, size),
		size: size,
	}
}

// Add 添加元素到环形缓冲区
func (rb *ringBuffer) Add(item time.Duration) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == rb.size {
		// 缓冲区满，覆盖最旧的元素
		rb.tail = (rb.tail + 1) % rb.size
	} else {
		rb.count++
	}

	rb.data[rb.head] = item
	rb.head = (rb.head + 1) % rb.size
}

// ToSlice 转换为 slice（用于计算平均值）
func (rb *ringBuffer) ToSlice() []time.Duration {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]time.Duration, rb.count)
	if rb.count < rb.size {
		// 缓冲区未满，直接复制
		copy(result, rb.data[:rb.count])
	} else {
		// 缓冲区满，需要分段复制
		n := copy(result, rb.data[rb.head:])
		copy(result[n:], rb.data[:rb.tail])
	}

	return result
}

// Len 返回当前元素数量
func (rb *ringBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Reset 重置环形缓冲区
func (rb *ringBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.head = 0
	rb.tail = 0
	rb.count = 0
}
