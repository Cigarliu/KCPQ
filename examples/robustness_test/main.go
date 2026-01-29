package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/kcpq/client"
)

func main() {
	fmt.Println("=== KCPQ v2.0 Robustness Test ===")
	fmt.Println("Testing: Memory leak & Goroutine leak fixes")
	fmt.Println("Server: 208.81.129.186:4000")
	fmt.Println("")

	// 记录初始状态
	var initialGoroutines = runtime.NumGoroutine()
	var initialMem runtime.MemStats
	runtime.ReadMemStats(&initialMem)

	fmt.Printf("Initial goroutines: %d\n", initialGoroutines)
	fmt.Printf("Initial memory: %.2f MB\n", float64(initialMem.Alloc)/1024/1024)
	fmt.Println("")

	// 创建客户端
	keyHex := os.Getenv("KCPQ_AES256_KEY_HEX")
	if keyHex == "" {
		log.Fatal("KCPQ_AES256_KEY_HEX is required (64 hex chars)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("invalid KCPQ_AES256_KEY_HEX: %v", err)
	}
	cli, err := client.Connect("208.81.129.186:4000", key)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer cli.Close()

	fmt.Println("Connected to server successfully")

	// 订阅主题
	msgChan, sub, err := cli.SubscribeChanWithContext(context.Background(), "test.robustness.>", 100)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Println("Subscribed to test.robustness.>")
	fmt.Println("")
	fmt.Println("Starting 3-minute test...")
	fmt.Println("Will publish messages and monitor resources...")
	fmt.Println("")

	// 定期发布消息和统计
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	statsTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()

	testDuration := 3 * time.Minute
	startTime := time.Now()

	// 测试循环
	for time.Since(startTime) < testDuration {
		select {
		case <-ticker.C:
			// 发布消息
			for i := 0; i < 10; i++ {
				subject := fmt.Sprintf("test.robustness.msg.%d", i)
				message := []byte(fmt.Sprintf("Test message %d at %s", i, time.Now().Format(time.RFC3339)))
				if err := cli.Publish(subject, message); err != nil {
					log.Printf("Publish error: %v", err)
				}
			}
			fmt.Printf("[%s] Published 10 messages\n", time.Now().Format("15:04:05"))

		case <-statsTicker.C:
			// 统计信息
			currentGoroutines := runtime.NumGoroutine()
			var currentMem runtime.MemStats
			runtime.ReadMemStats(&currentMem)

			fmt.Println("")
			fmt.Println("=== Statistics ===")
			fmt.Printf("Goroutines: %d (delta: %+d)\n", currentGoroutines, currentGoroutines-initialGoroutines)
			fmt.Printf("Memory: %.2f MB (delta: %+.2f MB)\n",
				float64(currentMem.Alloc)/1024/1024,
				(float64(currentMem.Alloc)-float64(initialMem.Alloc))/1024/1024)
			fmt.Printf("Running time: %v\n", time.Since(startTime).Round(time.Second))

			// 获取客户端统计
			stats := cli.GetStats()
			fmt.Printf("Client stats: Connected=%v, Sent=%d, Recv=%d\n",
				stats.Connected, stats.MessagesSent, stats.MessagesReceived)
			fmt.Println("")

		case msg := <-msgChan:
			// 接收消息
			if msg != nil {
				fmt.Printf("[RECV] %s: %s\n", msg.Subject, string(msg.Data))
			}

		case <-time.After(testDuration):
			goto done
		}
	}

done:
	fmt.Println("")
	fmt.Println("=== Test completed ===")
	fmt.Printf("Total duration: %v\n", time.Since(startTime).Round(time.Second))

	finalGoroutines := runtime.NumGoroutine()
	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)

	fmt.Printf("Final goroutines: %d (delta: %+d)\n", finalGoroutines, finalGoroutines-initialGoroutines)
	fmt.Printf("Final memory: %.2f MB (delta: %+.2f MB)\n",
		float64(finalMem.Alloc)/1024/1024,
		(float64(finalMem.Alloc)-float64(initialMem.Alloc))/1024/1024)

	stats := cli.GetStats()
	fmt.Printf("Client stats: Sent=%d, Recv=%d\n",
		stats.MessagesSent, stats.MessagesReceived)

	// 验证
	fmt.Println("")
	fmt.Println("=== Verification ===")
	if finalGoroutines-initialGoroutines > 5 {
		fmt.Printf("❌ FAIL: Goroutine leak detected (+%d goroutines)\n", finalGoroutines-initialGoroutines)
	} else {
		fmt.Println("✅ PASS: No goroutine leak")
	}

	memDeltaMB := (float64(finalMem.Alloc) - float64(initialMem.Alloc)) / 1024 / 1024
	if memDeltaMB > 10 {
		fmt.Printf("❌ FAIL: Memory leak detected (+%.2f MB)\n", memDeltaMB)
	} else {
		fmt.Println("✅ PASS: No memory leak")
	}

	fmt.Println("")
	fmt.Println("Test finished successfully!")
}
