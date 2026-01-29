package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kcpq/client"
)

func main() {
	addr := "localhost:4000"
	if addrEnv := os.Getenv("KCP_NATS_ADDR"); addrEnv != "" {
		addr = addrEnv
	}

	fmt.Printf("Connecting to %s...\n", addr)
	keyHex := os.Getenv("KCPQ_AES256_KEY_HEX")
	if keyHex == "" {
		log.Fatal("KCPQ_AES256_KEY_HEX is required (64 hex chars)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("invalid KCPQ_AES256_KEY_HEX: %v", err)
	}
	cli, err := client.Connect(addr, key)
	if err != nil {
		log.Fatalf("Connect failed: %v", err)
	}
	defer cli.Close()

	// 订阅所有 ping 请求（使用通配符）
	sub, err := cli.Subscribe("ping.>", func(msg *client.Message) {
		subject := msg.Subject

		// 检查是否是 ping 请求
		if strings.HasSuffix(subject, ".request") {
			// 提取 client ID
			parts := strings.Split(subject, ".")
			if len(parts) >= 3 {
				clientID := parts[1]

				// 构造响应主题
				responseSubject := fmt.Sprintf("ping.%s.response", clientID)

				// 立即回复（echo 原始数据）
				if err := cli.Publish(responseSubject, msg.Data); err != nil {
					log.Printf("[ERROR] Failed to send response to %s: %v", responseSubject, err)
				}
			}
		}
	})

	if err != nil {
		log.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Printf("Ping server listening on %s\n", addr)
	fmt.Printf("Subscribed to: ping.>\n")
	fmt.Printf("Ready to respond to ping requests...\n\n")

	// 保持运行
	select {}
}
