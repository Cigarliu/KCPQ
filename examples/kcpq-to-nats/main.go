package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kcpq/client"
	"github.com/nats-io/nats.go"
)

// ForwarderConfig 配置
type ForwarderConfig struct {
	KCPQServer       string        // KCPQ 服务器地址
	KCPQSubject      string        // KCPQ 订阅主题
	NATSServer       string        // NATS 服务器地址
	NATSSubject      string        // NATS 发布主题
	ReconnectDelay   time.Duration // 重连延迟（废弃，v2.0 使用自动重连）
	StatsInterval    time.Duration // 统计打印间隔
	AutoReconnect    bool          // 是否启用自动重连
	ReconnectInterval time.Duration // 自动重连间隔
	UseChannelMode   bool          // 是否使用 Channel 订阅模式（v2.0 推荐）
}

// ForwarderStats 统计信息
type ForwarderStats struct {
	TotalFrames uint64 // 总帧数
	TotalBytes  uint64 // 总字节数
	ErrorCount  uint64 // 错误次数
	StartTime   time.Time
}

// Forwarder KCPQ 到 NATS 转发器
type Forwarder struct {
	config        *ForwarderConfig
	stats         *ForwarderStats
	kcpClient     *client.Client
	natsConn      *nats.Conn
	subscription  *client.Subscription
	msgChan       <-chan *client.Message // v2.0: Channel 订阅模式
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewForwarder 创建转发器
func NewForwarder(config *ForwarderConfig) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())

	return &Forwarder{
		config: config,
		stats: &ForwarderStats{
			StartTime: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动转发器
func (f *Forwarder) Start() error {
	log.Println("===========================================")
	log.Println("KCPQ to NATS Forwarder v2.0")
	log.Println("===========================================")
	log.Printf("KCPQ Server: %s", f.config.KCPQServer)
	log.Printf("KCPQ Subject: %s", f.config.KCPQSubject)
	log.Printf("NATS Server: %s", f.config.NATSServer)
	log.Printf("NATS Subject: %s", f.config.NATSSubject)
	log.Printf("Auto Reconnect: %v", f.config.AutoReconnect)
	log.Printf("Channel Mode: %v", f.config.UseChannelMode)
	log.Println("===========================================\n")

	// 连接到 KCPQ（v2.0 API）
	if err := f.connectToKCPQ(); err != nil {
		return err
	}

	// 连接到 NATS
	if err := f.connectToNATS(); err != nil {
		return err
	}

	// 订阅 KCPQ（v2.0 API）
	if err := f.subscribeKCPQ(); err != nil {
		return err
	}

	// 启动统计打印
	go f.statsPrinter()

	log.Println("[OK] Forwarder started, waiting for messages...")

	return nil
}

// connectToKCPQ 连接到 KCPQ 服务器（v2.0 API）
func (f *Forwarder) connectToKCPQ() error {
	log.Printf("[INFO] Connecting to KCPQ: %s", f.config.KCPQServer)

	// v2.0: 使用 ConnectWithContext 支持 context
	cli, err := client.ConnectWithContext(f.ctx, f.config.KCPQServer)
	if err != nil {
		return err
	}

	f.kcpClient = cli
	log.Printf("[OK] Connected to KCPQ: %s", f.config.KCPQServer)

	// v2.0: 启用自动重连（替代手动重连逻辑）
	if f.config.AutoReconnect {
		interval := f.config.ReconnectInterval
		if interval == 0 {
			interval = 5 * time.Second // 默认 5 秒
		}
		f.kcpClient.EnableAutoReconnect(interval)
		log.Printf("[INFO] Auto-reconnect enabled (interval: %v)", interval)
	}

	return nil
}

// connectToNATS 连接到 NATS 服务器
func (f *Forwarder) connectToNATS() error {
	log.Printf("[INFO] Connecting to NATS: %s", f.config.NATSServer)

	// 连接 NATS，不使用账号密码
	nc, err := nats.Connect(f.config.NATSServer)
	if err != nil {
		return err
	}

	f.natsConn = nc
	log.Printf("[OK] Connected to NATS: %s", f.config.NATSServer)
	return nil
}

// subscribeKCPQ 订阅 KCPQ 主题（v2.0 API）
func (f *Forwarder) subscribeKCPQ() error {
	log.Printf("[INFO] Subscribing to KCPQ subject: %s", f.config.KCPQSubject)

	// v2.0: 优先使用 Channel 订阅模式（Go 惯用法）
	if f.config.UseChannelMode {
		msgChan, sub, err := f.kcpClient.SubscribeChanWithContext(f.ctx, f.config.KCPQSubject, 1000)
		if err != nil {
			return err
		}
		f.msgChan = msgChan
		f.subscription = sub
		log.Printf("[OK] Subscribed (Channel Mode) to: %s", f.config.KCPQSubject)

		// 启动消息处理 goroutine
		go f.handleMessagesFromChannel()
		return nil
	}

	// 回退到传统的回调模式（向后兼容）
	sub, err := f.kcpClient.SubscribeWithContext(f.ctx, f.config.KCPQSubject, func(msg *client.Message) {
		f.handleMessage(msg)
	})
	if err != nil {
		return err
	}

	f.subscription = sub
	log.Printf("[OK] Subscribed (Callback Mode) to: %s", f.config.KCPQSubject)
	return nil
}

// handleMessagesFromChannel 从 Channel 处理消息（v2.0 推荐）
func (f *Forwarder) handleMessagesFromChannel() {
	for {
		select {
		case <-f.ctx.Done():
			log.Println("[INFO] Message handler stopped")
			return
		case msg, ok := <-f.msgChan:
			if !ok {
				log.Println("[INFO] Message channel closed")
				return
			}
			f.handleMessage(msg)
		}
	}
}

// handleMessage 处理收到的消息
func (f *Forwarder) handleMessage(msg *client.Message) {
	// 解析消息（前8字节是时间戳，后面是 H.264 数据）
	if len(msg.Data) < 8 {
		log.Printf("[WARN] Invalid message: too short (%d bytes)", len(msg.Data))
		atomic.AddUint64(&f.stats.ErrorCount, 1)
		return
	}

	// 提取 H.264 数据（跳过8字节时间戳）
	h264Data := msg.Data[8:]

	// 发布到 NATS
	if err := f.natsConn.Publish(f.config.NATSSubject, h264Data); err != nil {
		log.Printf("[ERROR] Failed to publish to NATS: %v", err)
		atomic.AddUint64(&f.stats.ErrorCount, 1)
		return
	}

	// 更新统计
	atomic.AddUint64(&f.stats.TotalFrames, 1)
	atomic.AddUint64(&f.stats.TotalBytes, uint64(len(h264Data)))

	// 打印第一条消息
	if atomic.LoadUint64(&f.stats.TotalFrames) == 1 {
		log.Printf("[OK] First frame forwarded: %d bytes", len(h264Data))
	}
}

// statsPrinter 统计打印器
func (f *Forwarder) statsPrinter() {
	ticker := time.NewTicker(f.config.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			f.printStats()
		}
	}
}

// printStats 打印统计信息（v2.0 增强版）
func (f *Forwarder) printStats() {
	totalFrames := atomic.LoadUint64(&f.stats.TotalFrames)
	totalBytes := atomic.LoadUint64(&f.stats.TotalBytes)
	errorCount := atomic.LoadUint64(&f.stats.ErrorCount)

	elapsed := time.Since(f.stats.StartTime).Seconds()
	if elapsed == 0 {
		return
	}

	// 计算速率
	frameRate := float64(totalFrames) / elapsed
	byteRate := float64(totalBytes) / elapsed
	mbps := byteRate * 8 / 1000000 // Mbps
	avgFrameSize := float64(totalBytes) / float64(totalFrames) / 1024 // KB

	log.Println("===========================================")
	log.Println("Forwarder Statistics")
	log.Println("===========================================")
	log.Printf("Runtime: %.1f seconds", elapsed)
	log.Printf("\nFrames:")
	log.Printf("  Total: %d", totalFrames)
	log.Printf("  Rate: %.2f fps", frameRate)
	log.Printf("\nData:")
	log.Printf("  Total: %.2f MB", float64(totalBytes)/1024/1024)
	log.Printf("  Rate: %.2f MB/s", byteRate/1024/1024)
	log.Printf("  Bitrate: %.2f Mbps", mbps)
	log.Printf("\nFrame Info:")
	log.Printf("  Avg Size: %.2f KB", avgFrameSize)
	log.Printf("\nErrors:")
	log.Printf("  Count: %d", errorCount)
	if totalFrames > 0 {
		log.Printf("  Error Rate: %.4f%%", float64(errorCount)/float64(totalFrames)*100)
	}

	// v2.0: 显示 KCPQ 客户端统计信息
	if f.kcpClient != nil {
		stats := f.kcpClient.GetStats()
		log.Printf("\nKCPQ Client Stats (v2.0):")
		log.Printf("  Connected: %v", stats.Connected)
		log.Printf("  Messages Received: %d", stats.MessagesReceived)
		log.Printf("  Bytes Received: %d", stats.BytesReceived)
		log.Printf("  Avg Latency: %v", stats.AvgLatency)
		log.Printf("  Active Subscriptions: %d", stats.ActiveSubscriptions)
		log.Printf("  Reconnect Count: %d", stats.ReconnectCount)
		log.Printf("  Connection Errors: %d", stats.ConnectionErrors)
		log.Printf("  Subscription Errors: %d", stats.SubscriptionErrors)
	}

	log.Println("===========================================\n")
}

// Stop 停止转发器
func (f *Forwarder) Stop() {
	log.Println("[INFO] Shutting down forwarder...")
	f.cancel()

	if f.subscription != nil {
		f.subscription.Unsubscribe()
	}

	if f.kcpClient != nil {
		f.kcpClient.Close()
	}

	if f.natsConn != nil {
		f.natsConn.Close()
	}

	// 打印最终统计
	f.printStats()
	log.Println("[OK] Forwarder stopped")
}

func main() {
	// 配置
	config := &ForwarderConfig{
		KCPQServer:        "localhost:4000", // KCPQ 服务器（可通过环境变量 KCPQ_SERVER 覆盖）
		KCPQSubject:       "h264.stream",         // KCPQ 订阅主题
		NATSServer:        "nats://localhost:4222",     // NATS 服务器
		NATSSubject:       "h264.stream",         // NATS 发布主题
		ReconnectDelay:    3 * time.Second,       // 重连延迟（废弃）
		StatsInterval:     10 * time.Second,      // 每 10 秒打印统计
		AutoReconnect:     true,                  // v2.0: 启用自动重连
		ReconnectInterval: 5 * time.Second,       // v2.0: 自动重连间隔
		UseChannelMode:    true,                  // v2.0: 使用 Channel 订阅模式（推荐）
	}

	// 环境变量覆盖
	if kcpqAddr := os.Getenv("KCPQ_SERVER"); kcpqAddr != "" {
		config.KCPQServer = kcpqAddr
	}
	if kcpqSubj := os.Getenv("KCPQ_SUBJECT"); kcpqSubj != "" {
		config.KCPQSubject = kcpqSubj
	}
	if natsAddr := os.Getenv("NATS_SERVER"); natsAddr != "" {
		config.NATSServer = natsAddr
	}
	if natsSubj := os.Getenv("NATS_SUBJECT"); natsSubj != "" {
		config.NATSSubject = natsSubj
	}

	// 创建转发器
	forwarder := NewForwarder(config)

	// 启动转发器
	if err := forwarder.Start(); err != nil {
		log.Fatalf("[FATAL] Failed to start forwarder: %v", err)
	}

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 优雅关闭
	forwarder.Stop()
}
