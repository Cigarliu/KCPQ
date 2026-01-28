package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
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
	ctx                context.Context    // 用于取消操作的上下文
	cancel             context.CancelFunc // 取消函数
	subscriptions      []*Subscription
	mu                 sync.RWMutex
	done               chan struct{}
	closeOnce          sync.Once         // 确保 Close 只执行一次
	wg                 sync.WaitGroup    // 跟踪所有 goroutine（修复泄漏）
	receiveLoopDone    chan struct{}    // receiveLoop 退出时关闭
	heartbeatDone      chan struct{}    // heartbeat 退出时关闭
	onConnectionLost   ConnectionLostCallback // 连接断开回调
	pendingSubs        map[string]*subscriptionACK // 待确认的订阅
	pendingSubsMu      sync.RWMutex     // pendingSubs 的锁

	// 自动重连相关
	serverAddr         string           // 服务器地址
	autoReconnect      bool             // 是否启用自动重连
	reconnectInterval  time.Duration    // 重连间隔
	reconnecting       atomic.Bool      // 是否正在重连

	// 统计字段（使用原子操作）
	messagesSent       atomic.Int64     // 发送消息总数
	messagesReceived   atomic.Int64     // 接收消息总数
	bytesSent          atomic.Int64     // 发送字节总数
	bytesReceived      atomic.Int64     // 接收字节总数
	connectionErrors   atomic.Int64     // 连接错误数
	publishErrors      atomic.Int64     // 发布错误数
	subscriptionErrors atomic.Int64     // 订阅错误数
	totalSubscriptions atomic.Int64     // 总订阅数

	reconnectCount     atomic.Int64     // 重连次数
	// 延迟统计（需要互斥锁保护）
	latencyMu          sync.RWMutex     // 延迟统计的锁
	latencyBuffer    *ringBuffer   // 环形缓冲区（避免内存泄漏）
	lastLatency        time.Duration    // 最后一次延迟

	// 连接时间
	connectedAt        time.Time        // 连接时间
}

// ConnectWithContext 带上下文的连接（推荐使用）
// 支持超时、取消等 context 控制
func ConnectWithContext(ctx context.Context, addr string) (*Client, error) {
	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

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

	// 创建 context（基于传入的 context）
	clientCtx, cancel := context.WithCancel(ctx)

	client := &Client{
		conn:            conn,
		ctx:             clientCtx,
		cancel:          cancel,
		subscriptions:   make([]*Subscription, 0),
		done:            make(chan struct{}),
		receiveLoopDone: make(chan struct{}),
		heartbeatDone:   make(chan struct{}),
		pendingSubs:     make(map[string]*subscriptionACK),
		connectedAt:     time.Now(),
		latencyBuffer:   newRingBuffer(1000), // 环形缓冲区，1000个样本
		serverAddr:      addr,                         // 保存服务器地址
		autoReconnect:   false,                       // 默认不启用
		reconnectInterval: 5 * time.Second,
	}

	// 启动消息接收循环
	go client.receiveLoop()

	// 启动心跳
	go client.heartbeat()

	return client, nil
}

// Connect 连接到服务器
// Deprecated: 推荐使用 ConnectWithContext 以支持 context.Context
func Connect(addr string) (*Client, error) {
	return ConnectWithContext(context.Background(), addr)
}

// receiveLoop 接收消息循环
func (c *Client) receiveLoop() {
	c.wg.Add(1)              // 注册 goroutine
	defer c.wg.Done()        // 退出时注销
	defer close(c.receiveLoopDone) // 退出时关闭通道，通知应用层

	for {
		select {
		case <-c.done:
			log.Printf("[INFO] receiveLoop stopped by done channel")
			return
		case <-c.ctx.Done():
			log.Printf("[INFO] receiveLoop stopped by context")
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
	c.wg.Add(1)              // 注册 goroutine
	defer c.wg.Done()        // 退出时注销
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

// SubscribeWithContext 带上下文的订阅（推荐使用）
// 支持超时、取消等 context 控制
func (c *Client) SubscribeWithContext(ctx context.Context, subject string, callback MessageHandler) (*Subscription, error) {
	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 默认 channel capacity 为 100
	return c.SubscribeWithOptionsContext(ctx, subject, callback, 100)
}

// SubscribeWithOptionsContext 带配置和上下文的订阅（带ACK确认和重试）
func (c *Client) SubscribeWithOptionsContext(ctx context.Context, subject string, callback MessageHandler, channelCapacity int) (*Subscription, error) {
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
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

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

		// 等待ACK响应或超时（支持 context 取消）
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

		case <-ctx.Done():
			// context 取消
			c.pendingSubsMu.Lock()
			delete(c.pendingSubs, subject)
			c.pendingSubsMu.Unlock()

			c.removeSubscription(sub)
			sub.active = false
			close(sub.msgChan)

			return nil, ctx.Err()

		case <-c.done:
			return nil, fmt.Errorf("connection closed")
		}
	}

	return nil, fmt.Errorf("subscription failed after %d attempts: %w", maxRetries, lastErr)
}

// Subscribe 订阅主题（带ACK确认和重试）
// Deprecated: 推荐使用 SubscribeWithContext 以支持 context.Context
func (c *Client) Subscribe(subject string, callback MessageHandler) (*Subscription, error) {
	return c.SubscribeWithContext(context.Background(), subject, callback)
}

// SubscribeWithOptions 带配置的订阅（带ACK确认和重试）
// Deprecated: 推荐使用 SubscribeWithOptionsContext 以支持 context.Context
func (c *Client) SubscribeWithOptions(subject string, callback MessageHandler, channelCapacity int) (*Subscription, error) {
	return c.SubscribeWithOptionsContext(context.Background(), subject, callback, channelCapacity)
}

// SubscribeChan 订阅主题并返回 channel（推荐使用）
// 这是一种更符合 Go 惯用法的订阅方式
// msgChannel: 用于接收消息的 channel（只读）
// sub: 订阅对象，用于取消订阅
// error: 错误信息
func (c *Client) SubscribeChan(subject string, capacity int) (<-chan *Message, *Subscription, error) {
	return c.SubscribeChanWithContext(context.Background(), subject, capacity)
}

// SubscribeChanWithContext 带上下文的 Channel 订阅（推荐使用）
// 支持超时、取消等 context 控制
func (c *Client) SubscribeChanWithContext(ctx context.Context, subject string, capacity int) (<-chan *Message, *Subscription, error) {
	if capacity <= 0 {
		capacity = 100
	}

	msgChan := make(chan *Message, capacity)

	sub, err := c.SubscribeWithOptionsContext(ctx, subject, func(msg *Message) {
		// 非阻塞发送，防止 channel 满时阻塞
		select {
		case msgChan <- msg:
			// 成功发送到 channel
		default:
			// channel 满了，丢弃消息并记录警告
			log.Printf("[WARN] Channel full for subject %s, dropping message", subject)
		}
	}, capacity)

	if err != nil {
		close(msgChan)
		return nil, nil, err
	}

	return msgChan, sub, nil
}

// PublishWithContext 带上下文的发布消息（推荐使用）
// 支持超时、取消等 context 控制
func (c *Client) PublishWithContext(ctx context.Context, subject string, data []byte) error {
	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 支持写入超时
	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetWriteDeadline(deadline)
		defer c.conn.SetWriteDeadline(time.Time{}) // 重置 deadline
	}

	msg := protocol.NewMessageCmd(protocol.CmdPub, subject, data)
	encoded := msg.Encode()
	_, err := c.conn.Write(encoded)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}
	return nil
}

// Publish 发布消息
// Deprecated: 推荐使用 PublishWithContext 以支持 context.Context
func (c *Client) Publish(subject string, data []byte) error {
	return c.PublishWithContext(context.Background(), subject, data)
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
	var err error
	c.closeOnce.Do(func() {
		// 取消 context，触发所有监听 ctx.Done() 的操作
		if c.cancel != nil {
			c.cancel()
		}

		close(c.done)

		// 等待所有 goroutine 退出（修复泄漏）
		c.wg.Wait()

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
			err = c.conn.Close()
		}
	})

	return err
}

// GetStats 获取客户端统计
func (c *Client) GetStats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 计算活跃订阅数
	activeSubs := 0
	for _, sub := range c.subscriptions {
		if sub.active {
			activeSubs++
		}
	}

	// 计算运行时间和 QPS
	var uptime time.Duration
	if !c.connectedAt.IsZero() {
		uptime = time.Since(c.connectedAt)
	}

	var sentPerSec, receivedPerSec float64
	if uptime.Seconds() > 0 {
		sentPerSec = float64(c.messagesSent.Load()) / uptime.Seconds()
		receivedPerSec = float64(c.messagesReceived.Load()) / uptime.Seconds()
	}

	// 计算平均延迟
	c.latencyMu.RLock()
	avgLatency := c.calculateAvgLatencyLocked()
	lastLatency := c.lastLatency
	c.latencyMu.RUnlock()

	// 判断是否已连接
	connected := c.conn != nil

	return ClientStats{
		// 连接状态
		Connected:     connected,
		ConnectedAt:   c.connectedAt,

		// 消息统计
		MessagesSent:           c.messagesSent.Load(),
		MessagesReceived:       c.messagesReceived.Load(),
		MessagesSentPerSec:     sentPerSec,
		MessagesReceivedPerSec: receivedPerSec,

		// 网络统计
		BytesSent:    c.bytesSent.Load(),
		BytesReceived: c.bytesReceived.Load(),
		AvgLatency:   avgLatency,
		LastLatency:  lastLatency,

		// 订阅统计
		ActiveSubscriptions: activeSubs,
		TotalSubscriptions:  c.totalSubscriptions.Load(),

		// 错误统计
		ConnectionErrors:  c.connectionErrors.Load(),
		PublishErrors:     c.publishErrors.Load(),
		SubscriptionErrors: c.subscriptionErrors.Load(),
		ReconnectCount:       c.reconnectCount.Load(),
	}
}

// calculateAvgLatencyLocked 计算平均延迟（调用时必须持有 latencyMu 锁）
func (c *Client) calculateAvgLatencyLocked() time.Duration {
	latencies := c.latencyBuffer.ToSlice()
	if len(latencies) == 0 {
		return 0
	}

	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}

	return sum / time.Duration(len(latencies))
}

// recordLatency 记录延迟
func (c *Client) recordLatency(latency time.Duration) {
	c.latencyMu.Lock()
	defer c.latencyMu.Unlock()

	c.lastLatency = latency

		// 使用环形缓冲区（避免内存泄漏）
	c.latencyBuffer.Add(latency)
}

// ClientStats 客户端统计信息（增强版）
type ClientStats struct {
	// 连接状态
	Connected            bool      // 是否已连接
	ConnectedAt          time.Time // 连接时间
	DisconnectedAt       time.Time // 断开时间

	// 消息统计
	MessagesSent         int64   // 发送消息总数
	MessagesReceived     int64   // 接收消息总数
	MessagesSentPerSec   float64 // 发送消息速率（msg/s）
	MessagesReceivedPerSec float64 // 接收消息速率（msg/s）

	// 网络统计
	BytesSent            int64   // 发送字节总数
	BytesReceived        int64   // 接收字节总数
	AvgLatency           time.Duration // 平均延迟
	LastLatency          time.Duration // 最后一次延迟

	// 订阅统计
	ActiveSubscriptions  int     // 活跃订阅数
	TotalSubscriptions   int64   // 总订阅数

	// 错误统计
	ConnectionErrors     int64   // 连接错误数
	PublishErrors        int64   // 发布错误数
	SubscriptionErrors   int64   // 订阅错误数
	ReconnectCount        int64   // 重连次数
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

// ============== 自动重连功能 ==============

// EnableAutoReconnect 启用自动重连
// interval: 重连间隔（默认 5 秒）
func (c *Client) EnableAutoReconnect(interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.autoReconnect = true
	if interval > 0 {
		c.reconnectInterval = interval
	}

	log.Printf("[INFO] Auto-reconnect enabled (interval: %v)", c.reconnectInterval)
}

// DisableAutoReconnect 禁用自动重连
func (c *Client) DisableAutoReconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.autoReconnect = false
	log.Printf("[INFO] Auto-reconnect disabled")
}

// reconnect 执行重连并恢复订阅
func (c *Client) reconnect() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return // 已经在重连中
	}

	go func() {
		defer c.reconnecting.Store(false)

		for {
			// 检查是否应该停止重连
			c.mu.RLock()
			autoReconnect := c.autoReconnect
			c.mu.RUnlock()

			if !autoReconnect {
				return
			}

			select {
			case <-c.done:
				log.Printf("[INFO] Client closed, stop reconnecting")
				return
			default:
			}

			log.Printf("[INFO] Reconnecting to %s in %v...", c.serverAddr, c.reconnectInterval)
			time.Sleep(c.reconnectInterval)

			// 尝试重连
			err := c.doReconnect()
			if err != nil {
				c.connectionErrors.Add(1)
				c.reconnectCount.Add(1)
				log.Printf("[WARN] Reconnect failed: %v, will retry...", err)
				continue
			}

			log.Printf("[INFO] Reconnect successful!")
			c.connectedAt = time.Now()

			// 恢复所有订阅
			c.restoreSubscriptions()

			return
		}
	}()
}

// doReconnect 执行实际的重连操作
func (c *Client) doReconnect() error {
	// 步骤1: 关闭旧连接
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()

	// 步骤2: 等待所有旧 goroutine 退出（修复泄漏）
	c.wg.Wait()

	// 步骤3: 创建新的 done channel
	c.done = make(chan struct{})
	c.receiveLoopDone = make(chan struct{})
	c.heartbeatDone = make(chan struct{})

	// 步骤4: 建立新连接
	conn, err := kcp.DialWithOptions(c.serverAddr, nil, 10, 3)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	// 配置 KCP 参数
	conn.SetNoDelay(1, 10, 2, 1)
	conn.SetWindowSize(1024, 1024)
	conn.SetReadBuffer(4 * 1024 * 1024)
	conn.SetWriteBuffer(4 * 1024 * 1024)
	conn.SetWriteDelay(false)
	conn.SetStreamMode(false)

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// 步骤5: 重启接收循环（已经使用 WaitGroup）
	go c.receiveLoop()

	// 步骤6: 重启心跳（已经使用 WaitGroup）
	go c.heartbeat()

	return nil
}

// restoreSubscriptions 恢复所有订阅
func (c *Client) restoreSubscriptions() {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("[INFO] Restoring %d subscriptions...", len(c.subscriptions))

	for i, sub := range c.subscriptions {
		if !sub.active {
			continue
		}

		log.Printf("[INFO] Restoring subscription #%d: %s", i, sub.subject)

		// 重新订阅
		msg := protocol.NewMessageCmd(protocol.CmdSub, sub.subject, nil)
		encoded := msg.Encode()

		c.mu.Unlock()
		_, err := c.conn.Write(encoded)
		c.mu.Lock()

		if err != nil {
			log.Printf("[ERROR] Failed to restore subscription %s: %v", sub.subject, err)
			c.subscriptionErrors.Add(1)
			continue
		}

		log.Printf("[INFO] Subscription restored: %s", sub.subject)
	}

	log.Printf("[INFO] All subscriptions restored")
}

// triggerConnectionLost 触发连接断开处理（修改以支持自动重连）
func (c *Client) triggerConnectionLost() {
	log.Printf("[WARN] Connection lost detected")

	// 调用用户自定义的回调（如果有）
	if c.onConnectionLost != nil {
		go c.onConnectionLost()
	}

	// 如果启用了自动重连，触发重连
	c.mu.RLock()
	autoReconnect := c.autoReconnect
	c.mu.RUnlock()

	if autoReconnect {
		log.Printf("[INFO] Auto-reconnect triggered")
		go c.reconnect()
	}
}
