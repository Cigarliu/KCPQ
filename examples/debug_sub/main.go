package main

import (
	"fmt"
	"log"
	"time"

	"github.com/kcpq/client"
)

func main() {
	fmt.Println("连接服务器...")
	cli, err := client.Connect("localhost:4000")
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	fmt.Println("订阅 video.stream...")
	sub, err := cli.Subscribe("video.stream", func(msg *client.Message) {
		fmt.Printf("收到消息: subject=%s, len=%d\n", msg.Subject, len(msg.Data))
	})
	if err != nil {
		log.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Println("等待消息...")
	time.Sleep(2 * time.Second)

	fmt.Println("发送测试消息...")
	err = cli.Publish("video.stream", []byte("test message"))
	if err != nil {
		log.Printf("发布失败: %v", err)
	}

	fmt.Println("等待接收...")
	time.Sleep(2 * time.Second)

	stats := cli.GetStats()
	fmt.Printf("\n客户端统计: ActiveSubscriptions=%d\n", stats.ActiveSubscriptions)
}
