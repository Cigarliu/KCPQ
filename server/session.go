package server

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/kcpq/protocol"
	"github.com/xtaci/kcp-go"
)

// Session 代表一个客户端连接
type Session struct {
	conn     *kcp.UDPSession
	server   *Server
	subjects []string // 订阅的 subject 列表
	mu       sync.RWMutex
	lastPing time.Time
	outChan  chan []byte
}

// NewSession 创建新会话
func NewSession(conn *kcp.UDPSession, server *Server) *Session {
	return &Session{
		conn:     conn,
		server:   server,
		subjects: make([]string, 0),
		lastPing: time.Now(),
		outChan:  make(chan []byte, 10000), // 增大到 10000，支持高速视频流
	}
}

// Start 启动会话处理
func (s *Session) Start() {
	defer s.Close()

	s.conn.SetNoDelay(1, 20, 2, 1)
	s.conn.SetWindowSize(1024, 1024)
	s.conn.SetReadBuffer(4 * 1024 * 1024)
	s.conn.SetWriteBuffer(4 * 1024 * 1024)
	s.conn.SetWriteDelay(false)
	s.conn.SetStreamMode(false)

	// 设置读写超时
	s.conn.SetReadDeadline(time.Now().Add(120 * time.Second))

	// 写循环
	go func() {
		for data := range s.outChan {
			_, _ = s.conn.Write(data)
		}
	}()

	for {
		// 使用新的 ParseFromReader 直接从流中读取
		// 这样利用 KCP 内部的缓冲和重组，避免应用层手动 append 和拷贝
		msg, err := protocol.ParseFromReader(s.conn)
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
		s.conn.SetReadDeadline(time.Now().Add(120 * time.Second))
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
	s.subjects = append(s.subjects, subject)
	s.mu.Unlock()

	// 添加到路由器
	s.server.router.Subscribe(subject, s)

	fmt.Printf("[DEBUG] 订阅成功: subject=%s, total subjects=%d\n", subject, len(s.subjects))

	// 发送 OK 响应
	s.SendOK(fmt.Sprintf("subscribed to %s", subject))

	return nil
}

// handleUnsubscribe 处理取消订阅
func (s *Session) handleUnsubscribe(msg *protocol.Message) error {
	subject := msg.Subject

	s.mu.Lock()
	// 从列表中移除
	for i, sub := range s.subjects {
		if sub == subject {
			s.subjects = append(s.subjects[:i], s.subjects[i+1:]...)
			break
		}
	}
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
	msg := protocol.NewMessageCmd(protocol.CmdMsg, subject, payload)
	encoded := msg.Encode()
	select {
	case s.outChan <- encoded:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// SendEncoded 直接发送已编码的消息（复用缓冲区）
func (s *Session) SendEncoded(encoded []byte) error {
	select {
	case s.outChan <- encoded:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// SendOK 发送 OK 响应
func (s *Session) SendOK(text string) error {
	msg := protocol.NewMessageCmd(protocol.CmdOk, text, nil)
	encoded := msg.Encode()
	select {
	case s.outChan <- encoded:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// SendError 发送错误响应
func (s *Session) SendError(text string) error {
	msg := protocol.NewMessageCmd(protocol.CmdErr, text, nil)
	encoded := msg.Encode()
	select {
	case s.outChan <- encoded:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// SendPong 发送 PONG 响应
func (s *Session) SendPong() error {
	msg := protocol.NewMessageCmd(protocol.CmdPong, "", nil)
	encoded := msg.Encode()
	select {
	case s.outChan <- encoded:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// Close 关闭会话
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		// 从路由器移除所有订阅
		s.server.router.RemoveSession(s)

		s.conn.Close()
		s.conn = nil
	}
	if s.outChan != nil {
		close(s.outChan)
	}
}

// RemoteAddr 返回远程地址
func (s *Session) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// GetSubjects 返回订阅的 subjects
func (s *Session) GetSubjects() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subjects := make([]string, len(s.subjects))
	copy(subjects, s.subjects)
	return subjects
}
