package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/kcpq/client"
)

type PingPongTest struct {
	ClientID       string
	ServerAddr     string
	MessageCount   int
	MessageSize    int
	Results        []time.Duration
	mu             sync.Mutex
	ReceivedCount  int
}

func NewPingPongTest(clientID, serverAddr string, messageCount, messageSize int) *PingPongTest {
	return &PingPongTest{
		ClientID:     clientID,
		ServerAddr:   serverAddr,
		MessageCount: messageCount,
		MessageSize:  messageSize,
		Results:      make([]time.Duration, 0, messageCount),
	}
}

func (ppt *PingPongTest) Run() error {
	fmt.Printf("\n========================================\n")
	fmt.Printf("Ping-Pong Test: %s\n", ppt.ClientID)
	fmt.Printf("========================================\n")
	fmt.Printf("Server: %s\n", ppt.ServerAddr)
	fmt.Printf("Message Size: %d bytes\n", ppt.MessageSize)
	fmt.Printf("Count: %d\n", ppt.MessageCount)
	fmt.Printf("\n")

	// 连接服务器
	cli, err := client.Connect(ppt.ServerAddr)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer cli.Close()

	// 订阅响应
	respChan := make(chan *client.Message, 100)
	sub, err := cli.Subscribe(fmt.Sprintf("ping.%s.response", ppt.ClientID), func(msg *client.Message) {
		respChan <- msg
	})
	if err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}
	defer sub.Unsubscribe()

	// 等待连接建立
	time.Sleep(1 * time.Second)

	fmt.Printf("Starting ping-pong test...\n\n")

	// 发送 ping 并等待 pong
	for i := 0; i < ppt.MessageCount; i++ {
		// 构造 ping 消息
		pingMsg := make([]byte, ppt.MessageSize)
		binary.BigEndian.PutUint64(pingMsg[0:8], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint32(pingMsg[8:12], uint32(i))
		copy(pingMsg[12:], []byte(ppt.ClientID))

		// 发送前记录时间
		sendTime := time.Now()

		// 发送 ping
		err := cli.Publish(fmt.Sprintf("ping.%s.request", ppt.ClientID), pingMsg)
		if err != nil {
			log.Printf("[ERROR] Failed to send ping %d: %v", i, err)
			continue
		}

		// 等待响应（超时 5 秒）
		select {
		case <-respChan:
			// 收到响应，计算 RTT
			rtt := time.Since(sendTime)

			ppt.mu.Lock()
			ppt.Results = append(ppt.Results, rtt)
			ppt.ReceivedCount++
			ppt.mu.Unlock()

			// 每 10 个打印一次
			if (i+1)%10 == 0 || i == 0 {
				fmt.Printf("[Ping %d] RTT: %.2f ms\n", i+1, float64(rtt.Microseconds())/1000)
			}

		case <-time.After(5 * time.Second):
			log.Printf("[TIMEOUT] Ping %d timeout after 5 seconds", i)
		}

		// 间隔一下，避免太快
		time.Sleep(50 * time.Millisecond)
	}

	// 打印统计结果
	ppt.printStats()

	return nil
}

func (ppt *PingPongTest) printStats() {
	if len(ppt.Results) == 0 {
		fmt.Printf("\n========================================\n")
		fmt.Printf("No responses received!\n")
		fmt.Printf("========================================\n\n")
		return
	}

	// 计算统计数据
	var sum, min, max time.Duration
	min = ppt.Results[0]
	max = ppt.Results[0]

	for _, rtt := range ppt.Results {
		sum += rtt
		if rtt < min {
			min = rtt
		}
		if rtt > max {
			max = rtt
		}
	}

	avg := sum / time.Duration(len(ppt.Results))

	// 计算 P50, P95, P99
	sorted := make([]time.Duration, len(ppt.Results))
	copy(sorted, ppt.Results)
	// 简单排序
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 := sorted[len(sorted)*50/100]
	p95 := sorted[len(sorted)*95/100]
	p99 := sorted[len(sorted)*99/100]

	// 计算丢包率
	lossRate := float64(ppt.MessageCount-len(ppt.Results)) / float64(ppt.MessageCount) * 100

	fmt.Printf("\n========================================\n")
	fmt.Printf("Ping-Pong Test Results: %s\n", ppt.ClientID)
	fmt.Printf("========================================\n\n")

	fmt.Printf("Sent: %d\n", ppt.MessageCount)
	fmt.Printf("Received: %d\n", len(ppt.Results))
	fmt.Printf("Loss Rate: %.2f%%\n\n", lossRate)

	fmt.Printf("RTT Statistics:\n")
	fmt.Printf("  Min: %.2f ms\n", float64(min.Microseconds())/1000)
	fmt.Printf("  Max: %.2f ms\n", float64(max.Microseconds())/1000)
	fmt.Printf("  Avg: %.2f ms\n", float64(avg.Microseconds())/1000)
	fmt.Printf("  P50: %.2f ms\n", float64(p50.Microseconds())/1000)
	fmt.Printf("  P95: %.2f ms\n", float64(p95.Microseconds())/1000)
	fmt.Printf("  P99: %.2f ms\n", float64(p99.Microseconds())/1000)

	fmt.Printf("\nJitter (延迟抖动):\n")
	jitter := ppt.calculateJitter()
	fmt.Printf("  Avg Jitter: %.2f ms\n", jitter)

	fmt.Printf("\n========================================\n\n")
}

func (ppt *PingPongTest) calculateJitter() float64 {
	if len(ppt.Results) < 2 {
		return 0
	}

	var jitterSum float64
	for i := 1; i < len(ppt.Results); i++ {
		diff := float64(ppt.Results[i].Microseconds()-ppt.Results[i-1].Microseconds()) / 1000
		if diff < 0 {
			diff = -diff
		}
		jitterSum += diff
	}

	return jitterSum / float64(len(ppt.Results)-1)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	serverAddr := "142.171.156.96:4000"  // Remote server
	if addr := os.Getenv("KCP_NATS_ADDR"); addr != "" {
		serverAddr = addr
	}

	clientID := fmt.Sprintf("client-%d", rand.Intn(10000))
	messageCount := 100
	messageSize := 64 // 64 bytes

	ppt := NewPingPongTest(clientID, serverAddr, messageCount, messageSize)
	if err := ppt.Run(); err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}
