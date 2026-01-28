package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kcpq/client"
)

// RelayConfig 配置
type RelayConfig struct {
	UDPListenAddr    string        // UDP 监听地址
	KCPServerAddr    string        // KCPQ 服务器地址
	Subject          string        // 发布主题
	ReconnectDelay   time.Duration // 重连延迟（废弃，v2.0 使用自动重连）
	StatsInterval    time.Duration // 统计打印间隔
	BufferSize       int           // UDP 接收缓冲区大小
	AutoReconnect    bool          // 是否启用自动重连
	ReconnectInterval time.Duration // 自动重连间隔
}

// RelayStats 统计信息
type RelayStats struct {
	TotalFrames    uint64    // 总帧数
	TotalBytes     uint64    // 总字节数
	ErrorCount     uint64    // 错误次数
	StartTime      time.Time // 启动时间
	LastUpdateTime time.Time // 最后更新时间
}

// H264Relay H.264 转发器
type H264Relay struct {
	config      *RelayConfig
	stats       *RelayStats
	kcpClient   *client.Client
	ctx         context.Context
	cancel      context.CancelFunc
	initialized atomic.Bool
}

// NewH264Relay 创建 H.264 转发器
func NewH264Relay(config *RelayConfig) *H264Relay {
	ctx, cancel := context.WithCancel(context.Background())

	return &H264Relay{
		config: config,
		stats: &RelayStats{
			StartTime:      time.Now(),
			LastUpdateTime: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动转发器
func (r *H264Relay) Start() error {
	log.Println("===========================================")
	log.Println("H.264 to KCPQ Relay v2.0")
	log.Println("===========================================")
	log.Printf("UDP Listen: %s", r.config.UDPListenAddr)
	log.Printf("KCPQ Server: %s", r.config.KCPServerAddr)
	log.Printf("Subject: %s", r.config.Subject)
	log.Printf("Auto Reconnect: %v", r.config.AutoReconnect)
	log.Println("===========================================\n")

	// 连接到 KCPQ 服务器（使用 v2.0 API）
	if err := r.connectToKCPQ(); err != nil {
		return fmt.Errorf("failed to connect to KCPQ: %w", err)
	}

	// 启动统计打印
	go r.statsPrinter()

	// 启动 UDP 接收器
	go r.UDPReceiver()

	return nil
}

// connectToKCPQ 连接到 KCPQ 服务器（v2.0 API）
func (r *H264Relay) connectToKCPQ() error {
	log.Printf("[INFO] Connecting to KCPQ server: %s", r.config.KCPServerAddr)

	// 使用 ConnectWithContext 支持 context
	cli, err := client.ConnectWithContext(r.ctx, r.config.KCPServerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	r.kcpClient = cli
	r.initialized.Store(true)
	log.Printf("[OK] Connected to KCPQ server: %s", r.config.KCPServerAddr)

	// v2.0: 启用自动重连（替代手动重连逻辑和 ConnectionLostCallback）
	if r.config.AutoReconnect {
		interval := r.config.ReconnectInterval
		if interval == 0 {
			interval = 5 * time.Second // 默认 5 秒
		}
		r.kcpClient.EnableAutoReconnect(interval)
		log.Printf("[INFO] Auto-reconnect enabled (interval: %v)", interval)
	}

	return nil
}

// UDPReceiver UDP 接收器
func (r *H264Relay) UDPReceiver() {
	// 创建 UDP 监听
	addr, err := net.ResolveUDPAddr("udp", r.config.UDPListenAddr)
	if err != nil {
		log.Fatalf("[FATAL] Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Failed to listen UDP: %v", err)
	}
	defer conn.Close()

	// 设置接收缓冲区
	if err := conn.SetReadBuffer(r.config.BufferSize); err != nil {
		log.Printf("[WARN] Failed to set read buffer: %v", err)
	}

	log.Printf("[OK] UDP listener started on: %s", r.config.UDPListenAddr)
	log.Printf("[INFO] Buffer size: %d bytes", r.config.BufferSize)
	log.Printf("[INFO] Waiting for H.264 stream...\n")

	// 设置读取超时，以便定期检查 context
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))

	buffer := make([]byte, 65536) // 64KB max UDP packet

	for {
		select {
		case <-r.ctx.Done():
			log.Println("[INFO] UDP receiver stopped")
			return
		default:
		}

		// 接收 UDP 数据
		n, srcAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			// 检查是否是超时
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				continue
			}
			log.Printf("[ERROR] UDP read error: %v", err)
			atomic.AddUint64(&r.stats.ErrorCount, 1)
			continue
		}

		// 打印首次接收信息
		if r.stats.TotalFrames == 0 {
			log.Printf("[OK] First H.264 packet received from: %s", srcAddr.String())
			log.Printf("[INFO] Packet size: %d bytes\n", n)
		}

		// 复制数据（避免 buffer 复用问题）
		data := make([]byte, n)
		copy(data, buffer[:n])

		// 添加时间戳（8 字节）
		timestampBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(timestampBytes, uint64(time.Now().UnixNano()))

		// 组合消息: timestamp + h264 frame
		message := append(timestampBytes, data...)

		// 发布到 KCPQ（使用 v2.0 API）
		r.publishToKCPQ(message, n)
	}
}

// publishToKCPQ 发布到 KCPQ 服务器（v2.0 API）
func (r *H264Relay) publishToKCPQ(data []byte, frameSize int) {
	// 检查连接状态
	if !r.initialized.Load() || r.kcpClient == nil {
		log.Printf("[WARN] KCPQ not connected, dropping frame (%d bytes)", frameSize)
		atomic.AddUint64(&r.stats.ErrorCount, 1)
		return
	}

	// v2.0: 使用 PublishWithContext（支持 context）
	if err := r.kcpClient.PublishWithContext(r.ctx, r.config.Subject, data); err != nil {
		log.Printf("[ERROR] Failed to publish: %v", err)
		atomic.AddUint64(&r.stats.ErrorCount, 1)
		return
	}

	// DEBUG: 打印发送成功消息（每100帧打印一次）
	frameNum := atomic.AddUint64(&r.stats.TotalFrames, 1)
	atomic.AddUint64(&r.stats.TotalBytes, uint64(frameSize))
	if frameNum%100 == 1 {
		log.Printf("[DEBUG] Published frame #%d (%d bytes) to %s", frameNum, frameSize, r.config.Subject)
	}
}

// statsPrinter 统计打印器（v2.0 增强）
func (r *H264Relay) statsPrinter() {
	ticker := time.NewTicker(r.config.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.printStats()
		}
	}
}

// printStats 打印统计信息（v2.0 增强版）
func (r *H264Relay) printStats() {
	totalFrames := atomic.LoadUint64(&r.stats.TotalFrames)
	totalBytes := atomic.LoadUint64(&r.stats.TotalBytes)
	errorCount := atomic.LoadUint64(&r.stats.ErrorCount)

	elapsed := time.Since(r.stats.StartTime).Seconds()
	if elapsed == 0 {
		return
	}

	// 计算速率
	frameRate := float64(totalFrames) / elapsed
	byteRate := float64(totalBytes) / elapsed
	mbps := byteRate * 8 / 1000000 // Mbps
	avgFrameSize := float64(totalBytes) / float64(totalFrames) / 1024 // KB

	log.Println("===========================================")
	log.Println("H.264 Relay Statistics")
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
	if r.kcpClient != nil {
		stats := r.kcpClient.GetStats()
		log.Printf("\nKCPQ Client Stats (v2.0):")
		log.Printf("  Connected: %v", stats.Connected)
		log.Printf("  Messages Sent: %d", stats.MessagesSent)
		log.Printf("  Bytes Sent: %d", stats.BytesSent)
		log.Printf("  Avg Latency: %v", stats.AvgLatency)
		log.Printf("  Reconnect Count: %d", stats.ReconnectCount)
		log.Printf("  Connection Errors: %d", stats.ConnectionErrors)
		log.Printf("  Publish Errors: %d", stats.PublishErrors)
	}

	log.Println("===========================================\n")
}

// Stop 停止转发器
func (r *H264Relay) Stop() {
	log.Println("[INFO] Shutting down H.264 relay...")
	r.cancel()

	if r.kcpClient != nil {
		r.kcpClient.Close()
	}

	// 打印最终统计
	r.printStats()
	log.Println("[OK] H.264 relay stopped")
}

func main() {
	// 配置
	config := &RelayConfig{
		UDPListenAddr:     ":22345",              // 本地 UDP 监听地址
		KCPServerAddr:     "localhost:4000", // KCPQ 服务器地址（可通过环境变量 KCPQ_SERVER 覆盖）
		Subject:           "h264.stream",         // 发布主题
		ReconnectDelay:    3 * time.Second,       // 重连延迟（废弃）
		StatsInterval:     10 * time.Second,      // 每 10 秒打印统计
		BufferSize:        10 * 1024 * 1024,      // 10MB 接收缓冲区
		AutoReconnect:     true,                  // v2.0: 启用自动重连
		ReconnectInterval: 5 * time.Second,       // v2.0: 自动重连间隔
	}

	// 可以通过环境变量覆盖
	if addr := os.Getenv("KCPQ_SERVER"); addr != "" {
		config.KCPServerAddr = addr
	}
	if subject := os.Getenv("KCPQ_SUBJECT"); subject != "" {
		config.Subject = subject
	}
	if udpAddr := os.Getenv("UDP_LISTEN"); udpAddr != "" {
		config.UDPListenAddr = udpAddr
	}

	// 创建转发器
	relay := NewH264Relay(config)

	// 启动转发器
	if err := relay.Start(); err != nil {
		log.Fatalf("[FATAL] Failed to start relay: %v", err)
	}

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 优雅关闭
	relay.Stop()
}
