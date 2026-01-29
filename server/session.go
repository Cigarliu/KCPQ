package server

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/kcpq/protocol"
	"github.com/xtaci/kcp-go"
)

// Session 代表一个客户端连接
type Session struct {
	conn           *kcp.UDPSession
	server         *Server
	subjects       []string // 订阅的 subject 列表
	mu             sync.RWMutex
	lastPing       time.Time
	outChan        chan []byte
	queuedBytes    atomic.Int64
	maxQueuedBytes int64
	writeTimeout   time.Duration
	done           chan struct{}
	closed         atomic.Bool
	closeOnce      sync.Once
}

// NewSession 创建新会话
func NewSession(conn *kcp.UDPSession, server *Server) *Session {
	outCap := 1024
	maxQueued := int64(32 * 1024 * 1024)
	writeTimeout := 5 * time.Second
	if server != nil {
		server.mu.RLock()
		if server.sessionOutChanCap > 0 {
			outCap = server.sessionOutChanCap
		}
		if server.maxSessionQueuedBytes > 0 {
			maxQueued = server.maxSessionQueuedBytes
		}
		if server.sessionWriteTimeout > 0 {
			writeTimeout = server.sessionWriteTimeout
		}
		server.mu.RUnlock()
	}
	return &Session{
		conn:           conn,
		server:         server,
		subjects:       make([]string, 0),
		lastPing:       time.Now(),
		outChan:        make(chan []byte, outCap),
		maxQueuedBytes: maxQueued,
		writeTimeout:   writeTimeout,
		done:           make(chan struct{}),
	}
}

// Start 启动会话处理
func (s *Session) Start() {
	defer s.Close()

	conn := s.conn
	if conn == nil {
		return
	}

	conn.SetNoDelay(1, 20, 2, 1)
	conn.SetWindowSize(1024, 1024)
	conn.SetReadBuffer(4 * 1024 * 1024)
	conn.SetWriteBuffer(4 * 1024 * 1024)
	conn.SetWriteDelay(false)
	conn.SetStreamMode(false)

	// 设置读写超时
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))

	// 写循环
	go func() {
		for {
			select {
			case <-s.done:
				return
			case data := <-s.outChan:
				s.queuedBytes.Add(-int64(len(data)))
				if s.writeTimeout > 0 {
					conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
				}
				if _, err := conn.Write(data); err != nil {
					s.Close()
					return
				}
			}
		}
	}()

	for {
		select {
		case <-s.done:
			return
		default:
		}

		// 使用新的 ParseFromReader 直接从流中读取
		// 这样利用 KCP 内部的缓冲和重组，避免应用层手动 append 和拷贝
		msg, err := protocol.ParseFromReader(conn)
		if err != nil {
			// 如果是超时或连接关闭，退出
			// 注意：ReadFull 返回 EOF 或其他错误都意味着流中断
			return
		}

		if err := s.HandleMessage(msg); err != nil {
			s.SendError(fmt.Sprintf("handle error: %s", err))
		}

		// 更新最后活动时间
		s.mu.Lock()
		s.lastPing = time.Now()
		s.mu.Unlock()

		// 重置读超时
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	}
}

// HandleMessage 处理接收到的消息
func (s *Session) HandleMessage(msg *protocol.Message) error {
	_ = msg

	switch msg.Command {
	case protocol.CmdSub:
		return s.handleSubscribe(msg)

	case protocol.CmdUnsub:
		return s.handleUnsubscribe(msg)

	case protocol.CmdPub:
		return s.handlePublish(msg)

	case protocol.CmdPing:
		return s.handlePing()

	default:
		return fmt.Errorf("unknown command: %d", msg.Command)
	}
}

// handleSubscribe 处理订阅
func (s *Session) handleSubscribe(msg *protocol.Message) error {
	subject := msg.Subject

	fmt.Printf("[DEBUG] 收到订阅请求: subject=%s, from=%s\n", subject, s.RemoteAddr())

	s.mu.Lock()
	exists := false
	for _, sub := range s.subjects {
		if sub == subject {
			exists = true
			break
		}
	}
	if !exists {
		s.subjects = append(s.subjects, subject)
		s.server.router.Subscribe(subject, s)
	}
	s.mu.Unlock()

	fmt.Printf("[DEBUG] 订阅成功: subject=%s, total subjects=%d\n", subject, len(s.subjects))

	// 发送 OK 响应
	s.SendOK(fmt.Sprintf("subscribed to %s", subject))

	return nil
}

// handleUnsubscribe 处理取消订阅
func (s *Session) handleUnsubscribe(msg *protocol.Message) error {
	subject := msg.Subject

	s.mu.Lock()
	filtered := s.subjects[:0]
	for _, sub := range s.subjects {
		if sub != subject {
			filtered = append(filtered, sub)
		}
	}
	s.subjects = filtered
	s.mu.Unlock()

	// 从路由器移除
	s.server.router.Unsubscribe(subject, s)

	s.SendOK(fmt.Sprintf("unsubscribed from %s", subject))

	return nil
}

// handlePublish 处理发布消息
func (s *Session) handlePublish(msg *protocol.Message) error {
	subject := msg.Subject
	// 找到所有订阅者
	subscribers := s.server.router.FindSubscribers(subject)

	// 预编码一次，复用到所有订阅者，避免重复编码与内存分配
	encoded := protocol.NewMessageCmd(protocol.CmdMsg, msg.Subject, msg.Payload).Encode()
	for _, sub := range subscribers {
		if err := sub.SendEncoded(encoded); err != nil {
			log.Printf("[ERROR] Failed to enqueue to %s: %v", sub.RemoteAddr(), err)
		}
	}

	return nil
}

// handlePing 处理心跳
func (s *Session) handlePing() error {
	return s.SendPong()
}

// SendMessage 发送消息给客户端
func (s *Session) SendMessage(subject string, payload []byte) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	if len(subject) > protocol.MaxSubjectLen {
		return fmt.Errorf("subject too long: %d", len(subject))
	}
	if 1+2+len(subject)+len(payload) > protocol.MaxBodyLen {
		return fmt.Errorf("message too large")
	}
	msg := protocol.NewMessageCmd(protocol.CmdMsg, subject, payload)
	encoded := msg.Encode()
	return s.enqueueEncoded(encoded)
}

// SendEncoded 直接发送已编码的消息（复用缓冲区）
func (s *Session) SendEncoded(encoded []byte) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	if len(encoded) > protocol.MaxBodyLen+4 {
		return fmt.Errorf("encoded message too large")
	}
	return s.enqueueEncoded(encoded)
}

// SendOK 发送 OK 响应
func (s *Session) SendOK(text string) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	text = truncateUTF8(text, protocol.MaxSubjectLen)
	msg := protocol.NewMessageCmd(protocol.CmdOk, text, nil)
	encoded := msg.Encode()
	return s.enqueueEncoded(encoded)
}

// SendError 发送错误响应
func (s *Session) SendError(text string) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	text = truncateUTF8(text, protocol.MaxSubjectLen)
	msg := protocol.NewMessageCmd(protocol.CmdErr, text, nil)
	encoded := msg.Encode()
	return s.enqueueEncoded(encoded)
}

// SendPong 发送 PONG 响应
func (s *Session) SendPong() error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	msg := protocol.NewMessageCmd(protocol.CmdPong, "", nil)
	encoded := msg.Encode()
	return s.enqueueEncoded(encoded)
}

// Close 关闭会话
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)

		conn := s.conn

		if s.server != nil {
			s.server.router.RemoveSession(s)
			s.server.removeSession(s)
		}

		if conn != nil {
			conn.Close()
		}
	})
}

// RemoteAddr 返回远程地址
func (s *Session) RemoteAddr() net.Addr {
	conn := s.conn
	if conn == nil {
		return nil
	}
	return conn.RemoteAddr()
}

func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	out := s[:maxBytes]
	for !utf8.ValidString(out) && len(out) > 0 {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return ""
	}
	return out
}

func (s *Session) enqueueEncoded(encoded []byte) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	n := int64(len(encoded))
	for {
		current := s.queuedBytes.Load()
		next := current + n
		if s.maxQueuedBytes > 0 && next > s.maxQueuedBytes {
			go s.Close()
			return fmt.Errorf("backpressure limit exceeded")
		}
		if s.queuedBytes.CompareAndSwap(current, next) {
			break
		}
	}
	select {
	case s.outChan <- encoded:
		return nil
	default:
		s.queuedBytes.Add(-n)
		go s.Close()
		return fmt.Errorf("send queue full")
	}
}

// GetSubjects 返回订阅的 subjects
func (s *Session) GetSubjects() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subjects := make([]string, len(s.subjects))
	copy(subjects, s.subjects)
	return subjects
}
