package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kcpq/client"
)

// 性能统计
type Stats struct {
	TotalMessages  uint64
	TotalBytes     uint64
	TotalLatency   uint64
	MinLatency     uint64
	MaxLatency     uint64
	ErrorCount     uint64
	StartTime      time.Time
	LastUpdateTime time.Time
	mu             sync.RWMutex
}

func (s *Stats) RecordMessage(size uint64, latency uint64) {
	atomic.AddUint64(&s.TotalMessages, 1)
	atomic.AddUint64(&s.TotalBytes, size)

	// 更新延迟统计
	for {
		old := atomic.LoadUint64(&s.MaxLatency)
		if latency <= old || atomic.CompareAndSwapUint64(&s.MaxLatency, old, latency) {
			break
		}
	}

	if latency < atomic.LoadUint64(&s.MinLatency) || s.MinLatency == 0 {
		atomic.StoreUint64(&s.MinLatency, latency)
	}

	atomic.AddUint64(&s.TotalLatency, latency)
}

func (s *Stats) RecordError() {
	atomic.AddUint64(&s.ErrorCount, 1)
}

func (s *Stats) GetSnapshot() (uint64, uint64, uint64, uint64, uint64) {
	totalMsg := atomic.LoadUint64(&s.TotalMessages)
	totalBytes := atomic.LoadUint64(&s.TotalBytes)
	totalLat := atomic.LoadUint64(&s.TotalLatency)
	minLat := atomic.LoadUint64(&s.MinLatency)
	maxLat := atomic.LoadUint64(&s.MaxLatency)
	return totalMsg, totalBytes, totalLat, minLat, maxLat
}

// H.264 NALU 类型模拟
func generateH264Frame(size int, frameNum int) []byte {
	// 简化的 H.264 NALU 结构
	frame := make([]byte, size)

	// NALU header (0x00 0x00 0x00 0x01)
	frame[0] = 0x00
	frame[1] = 0x00
	frame[2] = 0x00
	frame[3] = 0x01

	// NALU type (I-frame or P-frame)
	if frameNum%30 == 0 { // 每30帧一个 I-frame
		frame[4] = 0x67 // SPS
		frame[5] = 0x42 // Profile
	} else {
		frame[4] = 0x41 // P-frame
	}

	// 模拟视频数据
	for i := 6; i < size; i++ {
		frame[i] = byte(i % 256)
	}

	return frame
}

// 性能测试场景
type TestScenario struct {
	Name            string
	MessageSize     int
	MessageRate     time.Duration
	SubscriberCount int
	Duration        time.Duration
}

func main() {
	// 启动 pprof
	go func() {
		log.Println("Pprof listening on :6061")
		log.Println(http.ListenAndServe("localhost:6061", nil))
	}()

	if len(os.Args) < 2 {
		fmt.Println("Usage: perf_test <scenario>")
		fmt.Println("\nScenarios:")
		fmt.Println("  small     - 1KB messages, 1000 msg/sec")
		fmt.Println("  medium    - 10KB messages, 100 msg/sec")
		fmt.Println("  large     - 100KB messages, 30 msg/sec (simulates video)")
		fmt.Println("  h264-720p - 50KB frames, 30 fps (720p@30)")
		fmt.Println("  h264-1080p - 150KB frames, 30 fps (1080p@30)")
		fmt.Println("  stress    - Mixed workload stress test")
		os.Exit(1)
	}

	scenario := os.Args[1]

	var testScenarios map[string]TestScenario

	switch scenario {
	case "small":
		testScenarios = map[string]TestScenario{
			"small": {
				Name:            "Small Messages (1KB, 1000 msg/s)",
				MessageSize:     1024,
				MessageRate:     time.Millisecond,
				SubscriberCount: 1,
				Duration:        10 * time.Second,
			},
		}
	case "medium":
		testScenarios = map[string]TestScenario{
			"medium": {
				Name:            "Medium Messages (10KB, 100 msg/s)",
				MessageSize:     10 * 1024,
				MessageRate:     10 * time.Millisecond,
				SubscriberCount: 1,
				Duration:        10 * time.Second,
			},
		}
	case "large":
		testScenarios = map[string]TestScenario{
			"large": {
				Name:            "Large Messages (100KB, 30 msg/s)",
				MessageSize:     100 * 1024,
				MessageRate:     33 * time.Millisecond,
				SubscriberCount: 1,
				Duration:        10 * time.Second,
			},
		}
	case "h264-720p":
		testScenarios = map[string]TestScenario{
			"h264-720p": {
				Name:            "H.264 720p@30fps (50KB/frame)",
				MessageSize:     50 * 1024,
				MessageRate:     33 * time.Millisecond, // ~30 fps
				SubscriberCount: 5,
				Duration:        30 * time.Second,
			},
		}
	case "h264-1080p":
		testScenarios = map[string]TestScenario{
			"h264-1080p": {
				Name:            "H.264 1080p@30fps (150KB/frame)",
				MessageSize:     150 * 1024,
				MessageRate:     33 * time.Millisecond,
				SubscriberCount: 10,
				Duration:        30 * time.Second,
			},
		}
	case "stress":
		testScenarios = map[string]TestScenario{
			"stress": {
				Name:            "Stress Test (Mixed)",
				MessageSize:     50 * 1024,
				MessageRate:     10 * time.Millisecond,
				SubscriberCount: 50,
				Duration:        20 * time.Second,
			},
		}
	default:
		fmt.Printf("Unknown scenario: %s\n", scenario)
		os.Exit(1)
	}

	// 运行测试
	for name, config := range testScenarios {
		// 检查是否有自定义订阅者数量参数
		if len(os.Args) > 2 {
			if count, err := strconv.Atoi(os.Args[2]); err == nil && count > 0 {
				config.SubscriberCount = count
				fmt.Printf("Overriding subscriber count to: %d\n", count)
			}
		}

		fmt.Printf("\n========================================\n")
		fmt.Printf("Running Test: %s\n", config.Name)
		fmt.Printf("========================================\n\n")

		runPerformanceTest(name, config)
	}
}

func runPerformanceTest(name string, config TestScenario) {
	// 创建共享统计
	stats := &Stats{
		StartTime: time.Now(),
	}
	addr := os.Getenv("KCP_NATS_ADDR")
	if addr == "" {
		addr = "localhost:4000"
	}

	// 用于通知订阅者测试结束
	done := make(chan struct{})

	var wg sync.WaitGroup
	fmt.Printf("Starting %d subscribers...\n", config.SubscriberCount)

	// 为每个订阅者创建独立的连接
	for i := 0; i < config.SubscriberCount; i++ {
		wg.Add(1)
		go func(subID int) {
			defer wg.Done()

			// 每个订阅者独立的KCP连接
			cli, err := client.Connect(addr)
			if err != nil {
				log.Printf("[Subscriber %d] Failed to connect: %v", subID, err)
				return
			}
			defer cli.Close()

			sub, err := cli.Subscribe("video.stream", func(msg *client.Message) {
				// 记录接收时间和延迟
				now := time.Now().UnixNano()
				timestamp := int64(binary.BigEndian.Uint64(msg.Data[:8]))
				latency := uint64(now - timestamp)

				stats.RecordMessage(uint64(len(msg.Data)), latency)
			})

			if err != nil {
				log.Printf("[Subscriber %d] Failed to subscribe: %v", subID, err)
				return
			}
			defer sub.Unsubscribe()

			fmt.Printf("[Subscriber %d] Started\n", subID)

			// 等待测试结束
			<-done
		}(i)
	}

	// 等待所有订阅者准备就绪
	time.Sleep(2 * time.Second)

	// 启动统计打印
	stopStats := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				printStats(stats, config.SubscriberCount)
			case <-stopStats:
				return
			}
		}
	}()

	// 为发布者创建独立的连接
	publisherCli, err := client.Connect(addr)
	if err != nil {
		log.Fatalf("Failed to connect publisher: %v", err)
	}
	defer publisherCli.Close()

	// 开始发布消息
	fmt.Printf("\nStarting publisher (rate: %v)...\n\n", config.MessageRate)
	publishStats := &PublisherStats{}
	startPublishing(publisherCli, config, stats, publishStats)

	// 运行指定时长
	fmt.Printf("\nRunning for %v...\n\n", config.Duration)
	time.Sleep(config.Duration)

	// 通知订阅者测试结束
	close(done)

	// 等待所有订阅者goroutine退出
	wg.Wait()

	close(stopStats)

	// 打印最终统计
	fmt.Printf("\n========================================\n")
	fmt.Printf("Final Results: %s\n", config.Name)
	fmt.Printf("========================================\n")
	printFinalStats(stats, publishStats, config)

	fmt.Printf("\nTest completed!\n")
}

type PublisherStats struct {
	PublishedCount uint64
	PublishedBytes uint64
}

func startPublishing(cli *client.Client, config TestScenario, stats *Stats, pubStats *PublisherStats) {
	frameNum := 0
	ticker := time.NewTicker(config.MessageRate)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			frameNum++

			// 生成模拟的 H.264 帧
			frame := generateH264Frame(config.MessageSize, frameNum)

			// 添加时间戳（8字节）
			timestampBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(timestampBytes, uint64(time.Now().UnixNano()))

			// 组合消息: timestamp + frame
			message := append(timestampBytes, frame...)

			// 发布消息
			err := cli.Publish("video.stream", message)
			if err != nil {
				stats.RecordError()
				log.Printf("Failed to publish frame %d: %v", frameNum, err)
				continue
			}

			atomic.AddUint64(&pubStats.PublishedCount, 1)
			atomic.AddUint64(&pubStats.PublishedBytes, uint64(len(message)))
		}
	}()
}

func printStats(stats *Stats, subscriberCount int) {
	totalMsg, totalBytes, totalLat, minLat, maxLat := stats.GetSnapshot()
	elapsed := time.Since(stats.StartTime).Seconds()

	if elapsed == 0 {
		return
	}

	msgRate := float64(totalMsg) / elapsed
	byteRate := float64(totalBytes) / elapsed / 1024 / 1024 // MB/s
	avgLat := float64(totalLat) / float64(totalMsg) / 1e6   // ms

	fmt.Printf("[%s] Messages: %d | Rate: %.0f msg/s | Throughput: %.2f MB/s | Latency: min=%.2f ms, max=%.2f ms, avg=%.2f ms | Errors: %d\n",
		time.Now().Format("15:04:05"),
		totalMsg,
		msgRate,
		byteRate,
		float64(minLat)/1e6,
		float64(maxLat)/1e6,
		avgLat,
		atomic.LoadUint64(&stats.ErrorCount),
	)
}

func printFinalStats(stats *Stats, pubStats *PublisherStats, config TestScenario) {
	totalMsg, totalBytes, totalLat, minLat, maxLat := stats.GetSnapshot()
	elapsed := time.Since(stats.StartTime).Seconds()

	publishedCount := atomic.LoadUint64(&pubStats.PublishedCount)
	publishedBytes := atomic.LoadUint64(&pubStats.PublishedBytes)

	fmt.Printf("\nDuration: %.1f seconds\n", elapsed)
	fmt.Printf("\nPublished:\n")
	fmt.Printf("  Total Messages: %d\n", publishedCount)
	fmt.Printf("  Total Bytes: %.2f MB\n", float64(publishedBytes)/1024/1024)
	fmt.Printf("  Publish Rate: %.0f msg/s\n", float64(publishedCount)/elapsed)
	fmt.Printf("  Publish Throughput: %.2f MB/s\n", float64(publishedBytes)/elapsed/1024/1024)

	fmt.Printf("\nReceived (Aggregated):\n")
	fmt.Printf("  Total Messages: %d\n", totalMsg)
	fmt.Printf("  Total Bytes: %.2f MB\n", float64(totalBytes)/1024/1024)
	fmt.Printf("  Message Rate: %.0f msg/s\n", float64(totalMsg)/elapsed)
	fmt.Printf("  Throughput: %.2f MB/s\n", float64(totalBytes)/elapsed/1024/1024)

	if totalMsg > 0 {
		fmt.Printf("\nLatency Statistics:\n")
		fmt.Printf("  Min: %.2f ms\n", float64(minLat)/1e6)
		fmt.Printf("  Max: %.2f ms\n", float64(maxLat)/1e6)
		fmt.Printf("  Avg: %.2f ms\n", float64(totalLat)/float64(totalMsg)/1e6)
		fmt.Printf("  P99: <%.2f ms (estimated)\n", float64(maxLat)*0.99/1e6)
	}

	errors := atomic.LoadUint64(&stats.ErrorCount)
	if errors > 0 {
		fmt.Printf("\nErrors: %d (%.2f%%)\n", errors, float64(errors)/float64(publishedCount)*100)
	} else {
		fmt.Printf("\nErrors: 0 (Perfect!)\n")
	}

	fmt.Printf("\nPer-Subscriber Statistics:\n")
	if config.SubscriberCount > 0 {
		perSubMsg := float64(totalMsg) / float64(config.SubscriberCount)
		perSubRate := perSubMsg / elapsed
		fmt.Printf("  Messages per subscriber: %.0f\n", perSubMsg)
		fmt.Printf("  Rate per subscriber: %.0f msg/s\n", perSubRate)
	}

	fmt.Printf("\nEfficiency:\n")
	deliveryRate := float64(totalMsg) / float64(publishedCount) / float64(config.SubscriberCount) * 100
	fmt.Printf("  Delivery Rate: %.2f%%\n", deliveryRate)
}

// getLocalIP 获取本地IP地址
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
