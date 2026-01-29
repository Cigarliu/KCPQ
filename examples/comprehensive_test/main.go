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
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║     KCPQ v2.0 - 综合功能测试                         ║")
	fmt.Println("║     Complete Feature Test Suite                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 测试 1: Context 支持
	testContextSupport()

	// 测试 2: Channel 订阅模式
	testChannelSubscription()

	// 测试 3: 自动重连
	testAutoReconnect()

	// 测试 4: 统计信息
	testStatistics()

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║     ✅ 所有功能测试完成！                              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
}

// testContextSupport 测试 Context 支持
func testContextSupport() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("测试 1: Context 支持（超时、取消）")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 带超时的连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cli, err := client.ConnectWithContext(ctx, "localhost:4000", mustAES256Key())
	if err != nil {
		log.Fatalf("❌ 连接失败: %v", err)
	}
	defer cli.Close()

	fmt.Println("  ✅ 带超时连接成功")

	// 带超时的订阅
	subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer subCancel()

	received := make(chan *client.Message, 1)
	sub, err := cli.SubscribeWithContext(subCtx, "test.context.*", func(msg *client.Message) {
		select {
		case received <- msg:
		default:
		}
	})

	if err != nil {
		log.Fatalf("❌ 订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Println("  ✅ 带超时订阅成功")

	// 发布测试消息
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
	err = cli.PublishWithContext(pubCtx, "test.context.hello", []byte("world"))
	pubCancel()

	if err != nil {
		log.Printf("  ❌ 发布失败: %v", err)
	} else {
		fmt.Println("  ✅ 带超时发布成功")
	}

	// 等待接收消息
	select {
	case <-received:
		fmt.Println("  ✅ 消息接收成功")
	case <-time.After(2 * time.Second):
		fmt.Println("  ⚠️  消息接收超时")
	}

	fmt.Println()
}

// testChannelSubscription 测试 Channel 订阅模式
func testChannelSubscription() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("测试 2: Channel 订阅模式（Go 惯用法）")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli, err := client.ConnectWithContext(ctx, "localhost:4000", mustAES256Key())
	if err != nil {
		log.Fatalf("❌ 连接失败: %v", err)
	}
	defer cli.Close()

	// 使用 Channel 订阅
	msgChan, sub, err := cli.SubscribeChan("test.channel.*", 10)
	if err != nil {
		log.Fatalf("❌ Channel 订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Println("  ✅ Channel 订阅创建成功")

	// 使用 select 接收消息
	go func() {
		for i := 0; i < 3; i++ {
			pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
			cli.PublishWithContext(pubCtx, "test.channel.msg", []byte(fmt.Sprintf("消息 %d", i+1)))
			pubCancel()
			time.Sleep(100 * time.Millisecond)
		}
	}()

	count := 0
	timeout := time.After(2 * time.Second)

loop:
	for {
		select {
		case msg := <-msgChan:
			count++
			fmt.Printf("  ✅ 收到消息: %s\n", msg.Data)
			if count >= 3 {
				break loop
			}
		case <-timeout:
			if count < 3 {
				fmt.Printf("  ⚠️  只收到 %d/3 条消息\n", count)
			}
			break loop
		}
	}

	fmt.Println()
}

// testAutoReconnect 测试自动重连
func testAutoReconnect() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("测试 3: 自动重连功能")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	cli, err := client.Connect("localhost:4000", mustAES256Key())
	if err != nil {
		log.Fatalf("❌ 连接失败: %v", err)
	}
	defer cli.Close()

	// 启用自动重连
	cli.EnableAutoReconnect(5 * time.Second)
	fmt.Println("  ✅ 自动重连已启用（5秒间隔）")

	// 订阅主题
	sub, err := cli.Subscribe("test.reconnect.*", func(msg *client.Message) {
		fmt.Printf("  ✅ 收到消息: %s\n", msg.Data)
	})

	if err != nil {
		log.Fatalf("❌ 订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	fmt.Println("  ✅ 订阅成功（重连后自动恢复）")

	// 模拟发布消息
	for i := 0; i < 2; i++ {
		err := cli.Publish("test.reconnect.hello", []byte(fmt.Sprintf("消息 %d", i+1)))
		if err != nil {
			log.Printf("  ❌ 发布失败: %v", err)
		} else {
			fmt.Printf("  ✅ 发布消息 %d\n", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 查看统计信息
	stats := cli.GetStats()
	fmt.Printf("  📊 连接状态: %v\n", stats.Connected)
	fmt.Printf("  📊 活跃订阅: %d\n", stats.ActiveSubscriptions)

	fmt.Println()
}

// testStatistics 测试统计信息
func testStatistics() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("测试 4: 统计信息功能")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli, err := client.ConnectWithContext(ctx, "localhost:4000", mustAES256Key())
	if err != nil {
		log.Fatalf("❌ 连接失败: %v", err)
	}
	defer cli.Close()

	// 订阅主题
	sub, err := cli.Subscribe("test.stats.*", func(msg *client.Message) {
		// 处理消息
	})

	if err != nil {
		log.Fatalf("❌ 订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	// 发布大量消息
	fmt.Println("  📤 发布 100 条消息...")
	for i := 0; i < 100; i++ {
		cli.Publish("test.stats.data", []byte("message"))
		if i%20 == 19 {
			fmt.Printf("    进度: %d%%\n", (i + 1))
		}
	}

	// 等待处理
	time.Sleep(500 * time.Millisecond)

	// 获取统计信息
	stats := cli.GetStats()

	fmt.Println("\n  📊 统计信息:")
	fmt.Printf("    已连接: %v\n", stats.Connected)
	fmt.Printf("    发送消息: %d\n", stats.MessagesSent)
	fmt.Printf("    接收消息: %d\n", stats.MessagesReceived)
	fmt.Printf("    发送速率: %.2f msg/s\n", stats.MessagesSentPerSec)
	fmt.Printf("    接收速率: %.2f msg/s\n", stats.MessagesReceivedPerSec)
	fmt.Printf("    发送字节: %d\n", stats.BytesSent)
	fmt.Printf("    接收字节: %d\n", stats.BytesReceived)
	fmt.Printf("    平均延迟: %v\n", stats.AvgLatency)
	fmt.Printf("    活跃订阅: %d\n", stats.ActiveSubscriptions)
	fmt.Printf("    总订阅数: %d\n", stats.TotalSubscriptions)
	fmt.Printf("    重连次数: %d\n", stats.ReconnectCount)
	fmt.Printf("    连接错误: %d\n", stats.ConnectionErrors)
	fmt.Printf("    发布错误: %d\n", stats.PublishErrors)
	fmt.Printf("    订阅错误: %d\n", stats.SubscriptionErrors)

	fmt.Println()
}

func mustAES256Key() []byte {
	keyHex := os.Getenv("KCPQ_AES256_KEY_HEX")
	if keyHex == "" {
		log.Fatal("KCPQ_AES256_KEY_HEX is required (64 hex chars)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("invalid KCPQ_AES256_KEY_HEX: %v", err)
	}
	if len(key) != 32 {
		log.Fatalf("KCPQ_AES256_KEY_HEX must decode to 32 bytes, got %d", len(key))
	}
	return key
}
