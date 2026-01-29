package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	// 订阅 test.hello
	testSub, err := cli.Subscribe("test.hello", func(msg *client.Message) {
		fmt.Printf("[RECEIVED] test.hello: %s\n", string(msg.Data))
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %s", err)
	}
	defer testSub.Unsubscribe()

	// 订阅 foo.* (通配符)
	fooSub, err := cli.Subscribe("foo.*", func(msg *client.Message) {
		fmt.Printf("[RECEIVED] foo.*: %s = %s\n", msg.Subject, string(msg.Data))
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %s", err)
	}
	defer fooSub.Unsubscribe()

	// 订阅 test.> (多段通配符)
	allTestSub, err := cli.Subscribe("test.>", func(msg *client.Message) {
		fmt.Printf("[RECEIVED] test.>: %s = %s\n", msg.Subject, string(msg.Data))
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %s", err)
	}
	defer allTestSub.Unsubscribe()

	fmt.Println("Subscribed to:")
	fmt.Println("  - test.hello")
	fmt.Println("  - foo.*")
	fmt.Println("  - test.>")
	fmt.Println("\nWaiting for messages... (Press Ctrl+C to exit)")

	// 打印统计信息
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stats := cli.GetStats()
			fmt.Printf("\n=== Client Stats ===\n")
			fmt.Printf("Active Subscriptions: %d\n", stats.ActiveSubscriptions)
			fmt.Printf("====================\n")
		}
	}()

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nUnsubscribing and closing connection...")
}
