package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kcpq/client"
)

func main() {
	// 连接到服务器
	keyHex := os.Getenv("KCPQ_AES256_KEY_HEX")
	if keyHex == "" {
		log.Fatal("KCPQ_AES256_KEY_HEX is required (64 hex chars)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("invalid KCPQ_AES256_KEY_HEX: %v", err)
	}
	cli, err := client.Connect("localhost:4000", key)
	if err != nil {
		log.Fatalf("Failed to connect: %s", err)
	}
	defer cli.Close()

	fmt.Println("Connected to KCP-NATS server")
	fmt.Println("Publishing messages to 'test.hello'...")
	fmt.Println("Press Ctrl+C to exit")

	// 定时发布消息
	ticker := time.NewTicker(1 * time.Second)
	messageCount := 0

	defer ticker.Stop()

	for range ticker.C {
		messageCount++
		message := fmt.Sprintf("Message #%d at %s", messageCount, time.Now().Format(time.RFC3339))

		err := cli.Publish("test.hello", []byte(message))
		if err != nil {
			log.Printf("Failed to publish: %s", err)
			continue
		}

		fmt.Printf("[PUBLISHED] test.hello: %s\n", message)

		// 每10条消息发布到不同主题
		if messageCount%10 == 0 {
			fooMessage := fmt.Sprintf("Foo message #%d", messageCount)
			cli.Publish("foo.bar", []byte(fooMessage))
			fmt.Printf("[PUBLISHED] foo.bar: %s\n", fooMessage)
		}
	}
}
