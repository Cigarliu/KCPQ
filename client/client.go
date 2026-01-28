package client

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kcpq/protocol"
	"github.com/xtaci/kcp-go"
)

// ConnectionState 连接状态
type ConnectionState int

const (
	StateConnected ConnectionState = iota
	StateDisconnected
)

// ConnectionLostCallback 连接断开回调函数
type ConnectionLostCallback func()

// subscriptionACK 订阅确认信息
type subscriptionACK struct {
	subject    string
	acked      chan struct{}
	ackedAt    time.Time
	timeout    time.Time
}

// Client KCP-NATS 客户端（增强版）
type Client struct {
	conn               *kcp.UDPSession
	subscriptions      []*Subscription
	mu                 sync.RWMutex
	done               chan struct{}
	receiveLoopDone    chan struct{}    // receiveLoop 退出时关闭
	heartbeatDone      chan struct{}    // heartbeat 退出时关闭
	onConnectionLost   ConnectionLostCallback // 连接断开回调
	pendingSubs        map[string]*subscriptionACK // 待确认的订阅
	pendingSubsMu      sync.RWMutex     // pendingSubs 的锁
}

// Connect 连接到服务器
func Connect(addr string) (*Client, error) {
	conn, err := kcp.DialWithOptions(addr, nil, 10, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	// 设置 KCP 参数为低延迟模式
	conn.SetNoDelay(1, 10, 2, 1)
	conn.SetWindowSize(1024, 1024)
	conn.SetReadBuffer(4 * 1024 * 1024)
	conn.SetWriteBuffer(4 * 1024 * 1024)

	// 关键修复：禁用写延迟和流模式
	conn.SetWriteDelay(false)
	conn.SetStreamMode(false)

	client := &Client{
		conn:            conn,
		subscriptions:   make([]*Subscription, 0),
		done:            make(chan struct{}),
		receiveLoopDone: make(chan struct{}),
		heartbeatDone:   make(chan struct{}),
		pendingSubs:     make(map[string]*subscriptionACK),
	}

	// 启动消息接收循环
	go client.receiveLoop()

	// 启动心跳
	go client.heartbeat()

	return client, nil
}

// receiveLoop 接收消息循环
func (c *Client) receiveLoop() {
	defer close(c.receiveLoopDone) // 退出时关闭通道，通知应用层

	for {
		select {
		case <-c.done:
			return
		default:
			c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))

			msg, err := protocol.ParseFromReader(c.conn)
			if err != nil {
				if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("Read error: %s", err)
				}
				return
			}

			c.handleMessage(msg)
		}
	}
}

// handleMessage 处理接收到的消息
func (c *Client) handleMessage(msg *protocol.Message) {
	switch msg.Command {
	case protocol.CmdMsg:
		c.mu.RLock()
		defer c.mu.RUnlock()

		for _, sub := range c.subscriptions {
			if sub.active && matchSubject(sub.subject, msg.Subject) {
				message := &Message{
					Subject: msg.Subject,
					Data:    msg.Payload,
				}

				if !sub.TryEnqueue(message) {
					log.Printf("[WARN] Dropped message for %s (queue full)", sub.subject)
				}
			}
		}

	case protocol.CmdOk:
		log.Printf("[OK] %s", msg.Subject)

		// 处理订阅ACK确认
		c.pendingSubsMu.Lock()

		// 直接匹配 subject
		if ack, exists := c.pendingSubs[msg.Subject]; exists {
			select {
			case <-ack.acked:
				// 已确认
			default:
				close(ack.acked)
				ack.ackedAt = time.Now()
				log.Printf("[DEBUG] Subscription ACK confirmed: %s", msg.Subject)
			}
			delete(c.pendingSubs, msg.Subject)
		} else if strings.Contains(msg.Subject, "subscribed to ") {
			// 处理 "subscribed to xxx" 格式
			subject := strings.TrimPrefix(msg.Subject, "subscribed to ")
			if ack, exists := c.pendingSubs[subject]; exists {
				select {
				case <-ack.acked:
					// 已确认
				default:
					close(ack.acked)
					ack.ackedAt = time.Now()
					log.Printf("[DEBUG] Subscription ACK confirmed: %s", subject)
				}
				delete(c.pendingSubs, subject)
			}
		}

		c.pendingSubsMu.Unlock()

	case protocol.CmdErr:
		log.Printf("[ERROR] %s", msg.Subject)

	case protocol.CmdPong:
		// 心跳响应
	}
}

// heartbeat 心跳
func (c *Client) heartbeat() {
	defer close(c.heartbeatDone)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			msg := protocol.NewMessageCmd(protocol.CmdPing, "", nil)
			encoded := msg.Encode()
			if _, err := c.conn.Write(encoded); err != nil {
				log.Printf("Heartbeat error: %s", err)
				return
			}
		}
	}
}

// Subscribe 订阅主题（带ACK确认和重试）
func (c *Client) Subscribe(subject string, callback MessageHandler) (*Subscription, error) {
	return c.SubscribeWithOptions(subject, callback, 100)
}

// SubscribeWithOptions 带配置的订阅（带ACK确认和重试）
func (c *Client) SubscribeWithOptions(subject string, callback MessageHandler, channelCapacity int) (*Subscription, error) {
	if channelCapacity <= 0 {
		channelCapacity = 100
	}

	const (
		maxRetries = 3
		ackTimeout = 5 * time.Second
		retryDelay = 1 * time.Second
	)

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[INFO] Retrying subscription to %s (attempt %d/%d)...", subject, attempt, maxRetries)
			time.Sleep(retryDelay)
		}

		sub := &Subscription{
			client:   c,
			subject:  subject,
			callback: callback,
			active:   true,
			msgChan:  make(chan *Message, channelCapacity),
		}

		sub.startProcessing()

		c.mu.Lock()
		c.subscriptions = append(c.subscriptions, sub)
		c.mu.Unlock()

		// 创建ACK等待通道
		ackChan := make(chan struct{})

		// 注册待确认的订阅
		c.pendingSubsMu.Lock()
		c.pendingSubs[subject] = &subscriptionACK{
			subject: subject,
			acked:   ackChan,
			timeout: time.Now().Add(ackTimeout),
		}
		c.pendingSubsMu.Unlock()

		// 发送订阅命令
		msg := protocol.NewMessageCmd(protocol.CmdSub, subject, nil)
		encoded := msg.Encode()
		_, err := c.conn.Write(encoded)
		if err != nil {
			c.pendingSubsMu.Lock()
			delete(c.pendingSubs, subject)
			c.pendingSubsMu.Unlock()

			c.removeSubscription(sub)
			sub.active = false
			close(sub.msgChan)

			lastErr = fmt.Errorf("failed to send SUB command: %w", err)
			log.Printf("[ERROR] %v", lastErr)
			continue
		}

		log.Printf("[DEBUG] Sent SUB command for %s, waiting for ACK...", subject)

		// 等待ACK响应或超时
		select {
		case <-ackChan:
			// 收到ACK
			log.Printf("[OK] Successfully subscribed to %s", subject)
			return sub, nil

		case <-time.After(ackTimeout):
			// 超时
			c.pendingSubsMu.Lock()
			delete(c.pendingSubs, subject)
			c.pendingSubsMu.Unlock()

			lastErr = fmt.Errorf("subscription ACK timeout after %v", ackTimeout)
			log.Printf("[WARN] Subscription to %s timed out waiting for ACK", subject)

			c.removeSubscription(sub)
			sub.active = false
			close(sub.msgChan)

			continue

		case <-c.done:
			return nil, fmt.Errorf("connection closed")
		}
	}

	return nil, fmt.Errorf("subscription failed after %d attempts: %w", maxRetries, lastErr)
}

// Publish 发布消息
func (c *Client) Publish(subject string, data []byte) error {
	msg := protocol.NewMessageCmd(protocol.CmdPub, subject, data)
	encoded := msg.Encode()
	_, err := c.conn.Write(encoded)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}
	return nil
}

// removeSubscription 移除订阅
func (c *Client) removeSubscription(sub *Subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, s := range c.subscriptions {
		if s == sub {
			c.subscriptions = append(c.subscriptions[:i], c.subscriptions[i+1:]...)
			break
		}
	}
}

// Close 关闭连接
func (c *Client) Close() error {
	close(c.done)

	c.mu.Lock()
	for _, sub := range c.subscriptions {
		sub.active = false
	}
	c.subscriptions = nil
	c.mu.Unlock()

	// 清理待确认的订阅
	c.pendingSubsMu.Lock()
	for subject, ack := range c.pendingSubs {
		select {
		case <-ack.acked:
		default:
			close(ack.acked)
		}
		delete(c.pendingSubs, subject)
	}
	c.pendingSubsMu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// GetStats 获取客户端统计
func (c *Client) GetStats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	activeSubs := 0
	for _, sub := range c.subscriptions {
		if sub.active {
			activeSubs++
		}
	}

	return ClientStats{
		ActiveSubscriptions: activeSubs,
	}
}

// ClientStats 客户端统计信息
type ClientStats struct {
	ActiveSubscriptions int
}

// SetConnectionLostCallback 设置连接断开回调函数
func (c *Client) SetConnectionLostCallback(callback ConnectionLostCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnectionLost = callback
}

// MonitorConnection 监控连接状态
func (c *Client) MonitorConnection() {
	select {
	case <-c.receiveLoopDone:
		log.Printf("[WARN] Connection lost: receiveLoop exited")
		c.triggerConnectionLost()
	case <-c.heartbeatDone:
		log.Printf("[WARN] Connection lost: heartbeat exited")
		c.triggerConnectionLost()
	case <-c.done:
		// 正常关闭
		return
	}
}

func (c *Client) triggerConnectionLost() {
	if c.onConnectionLost != nil {
		go c.onConnectionLost()
	}
}

// matchSubject 检查 subject 是否匹配 pattern（支持通配符）
func matchSubject(pattern, subject string) bool {
	if pattern == subject {
		return true
	}

	if strings.HasSuffix(pattern, ".>") {
		prefix := strings.TrimSuffix(pattern, ".>")
		if strings.HasPrefix(subject, prefix+".") || subject == prefix {
			return true
		}
	}

	if strings.Contains(pattern, "*") {
		patternParts := strings.Split(pattern, ".")
		subjectParts := strings.Split(subject, ".")

		if len(patternParts) != len(subjectParts) {
			return false
		}

		for i := 0; i < len(patternParts); i++ {
			if patternParts[i] == "*" {
				continue
			}
			if patternParts[i] != subjectParts[i] {
				return false
			}
		}
		return true
	}

	return false
}
