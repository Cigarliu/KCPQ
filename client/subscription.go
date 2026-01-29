package client

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kcpq/protocol"
)

// Subscription 表示一个订阅
type Subscription struct {
	client         *Client
	subject        string
	callback       MessageHandler
	active         atomic.Bool
	msgChan        chan *Message
	wg             sync.WaitGroup
	processedCount atomic.Int64
	droppedCount   atomic.Int64
	closeOnce      sync.Once // 防止重复关闭 channel
}

// MessageHandler 消息处理函数
type MessageHandler func(msg *Message)

// Message 表示收到的消息
type Message struct {
	Subject string
	Data    []byte
}

// SubscriptionStats 订阅统计信息
type SubscriptionStats struct {
	ProcessedCount int64
	DroppedCount   int64
	Active         bool
}

// startProcessing 启动消息处理goroutine
func (s *Subscription) startProcessing() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for msg := range s.msgChan {
			startTime := time.Now()
			s.callback(msg)
			elapsed := time.Since(startTime)

			s.processedCount.Add(1)

			// 如果callback处理太慢，打印警告
			if elapsed > 100*time.Millisecond {
				log.Printf("[WARN] Subscription %s: slow callback %v", s.subject, elapsed)
			}
		}
		log.Printf("[DEBUG] Subscription %s: processor stopped", s.subject)
	}()
}

// TryEnqueue 尝试将消息放入队列（非阻塞）
func (s *Subscription) TryEnqueue(msg *Message) bool {
	if !s.active.Load() {
		return false
	}

	defer func() {
		_ = recover()
	}()
	select {
	case s.msgChan <- msg:
		return true
	default:
		// 队列满，丢弃消息
		s.droppedCount.Add(1)
		return false
	}
}

// GetStats 获取订阅统计信息
func (s *Subscription) GetStats() SubscriptionStats {
	return SubscriptionStats{
		ProcessedCount: s.processedCount.Load(),
		DroppedCount:   s.droppedCount.Load(),
		Active:         s.active.Load(),
	}
}

// Unsubscribe 取消订阅
func (s *Subscription) Unsubscribe() error {
	// 使用 closeOnce 确保只执行一次
	var err error
	s.closeOnce.Do(func() {
		if !s.active.Load() {
			return
		}

		s.active.Store(false)

		// 关闭消息channel，停止处理goroutine
		close(s.msgChan)

		// 等待处理goroutine退出
		s.wg.Wait()

		msg := protocol.NewMessageCmd(protocol.CmdUnsub, s.subject, nil)
		encoded := msg.Encode()
		s.client.mu.RLock()
		conn := s.client.conn
		writeTimeout := s.client.writeTimeout
		s.client.mu.RUnlock()
		if conn == nil {
			err = nil
			s.client.removeSubscription(s)
			return
		}
		conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		_, err = conn.Write(encoded)

		if err == nil {
			s.client.removeSubscription(s)
		}

		log.Printf("[DEBUG] Unsubscribed from %s (processed: %d, dropped: %d)",
			s.subject, s.processedCount.Load(), s.droppedCount.Load())
	})

	return err
}

// IsActive 返回订阅是否活跃
func (s *Subscription) IsActive() bool {
	return s.active.Load()
}
