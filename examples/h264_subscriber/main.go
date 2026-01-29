package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kcpq/client"
)

func main() {
	fmt.Println("=== H.264 Stream Subscriber ===")
	fmt.Println("Server: 208.81.129.186:4000")
	fmt.Println("Subject: h264.stream")
	fmt.Println("")

	// 连接到 KCPQ 服务器
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

	fmt.Println("Connected to KCPQ server successfully")

	// 订阅 H.264 流
	msgChan, sub, err := cli.SubscribeChanWithContext(context.Background(), "h264.stream", 1000)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Println("Subscribed to h264.stream")
	fmt.Println("Waiting for H.264 frames...")
	fmt.Println("")

	// 统计
	frameCount := 0
	totalBytes := 0
	startTime := time.Now()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 每 10 秒打印统计
			elapsed := time.Since(startTime).Seconds()
			fps := float64(frameCount) / elapsed
			mbps := float64(totalBytes) * 8 / elapsed / 1000000

			fmt.Printf("[%s] Stats: %d frames, %.2f fps, %.2f MB, %.2f Mbps\n",
				time.Now().Format("15:04:05"),
				frameCount,
				fps,
				float64(totalBytes)/1024/1024,
				mbps)

		case msg, ok := <-msgChan:
			if !ok {
				fmt.Println("Channel closed")
				return
			}

			if msg != nil {
				frameCount++
				totalBytes += len(msg.Data)

				// 每收到 100 帧打印一次
				if frameCount%100 == 1 {
					fmt.Printf("[%s] Frame #%d: %d bytes, Subject: %s\n",
						time.Now().Format("15:04:05"),
						frameCount,
						len(msg.Data),
						msg.Subject)
				}
			}
		}
	}
}
