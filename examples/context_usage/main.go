package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/kcpq/client"
)

func main() {
	fmt.Println("=== KCPQ Context API 示例 ===\n")

	// 示例 1: 带超时的连接
	fmt.Println("示例 1: 带超时的连接")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel1()

	cli, err := client.ConnectWithContext(ctx1, "localhost:4000")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Fatalf("连接超时: %v", err)
		}
		log.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()
	fmt.Printf("✅ 连接成功\n\n")

	// 示例 2: 带超时的订阅
	fmt.Println("示例 2: 带超时的订阅")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	msgCount := 0
	sub, err := cli.SubscribeWithContext(ctx2, "demo.*", func(msg *client.Message) {
		msgCount++
		fmt.Printf("  收到消息 #%d: %s = %s\n", msgCount, msg.Subject, string(msg.Data))
	})
	if err != nil {
		log.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()
	fmt.Printf("✅ 订阅成功\n\n")

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 示例 3: 带超时的发布
	fmt.Println("示例 3: 带超时的发布消息")
	for i := 1; i <= 5; i++ {
		ctx3, cancel3 := context.WithTimeout(context.Background(), 1*time.Second)
		err := cli.PublishWithContext(ctx3, "demo.hello", []byte(fmt.Sprintf("消息 %d", i)))
		cancel3()

		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("  ⚠️  发布超时: %v", err)
			} else {
				log.Printf("  ❌ 发布失败: %v", err)
			}
		} else {
			fmt.Printf("  ✅ 发布消息 %d\n", i)
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println()

	// 等待消息处理完成
	time.Sleep(500 * time.Millisecond)

	// 示例 4: Context 取消
	fmt.Println("示例 4: Context 取消")
	ctx4, cancel4 := context.WithCancel(context.Background())

	go func() {
		time.Sleep(1 * time.Second)
		fmt.Println("  取消 Context...")
		cancel4()
	}()

	sub2, err := cli.SubscribeWithContext(ctx4, "cancel.*", func(msg *client.Message) {
		fmt.Printf("  收到消息: %s\n", msg.Subject)
	})
	if errors.Is(err, context.Canceled) {
		fmt.Println("  ✅ Context 被成功取消")
	} else if err != nil {
		log.Printf("  ❌ 订阅失败: %v", err)
	} else {
		sub2.Unsubscribe()
	}
	fmt.Println()

	// 示例 5: 可取消的发布
	fmt.Println("示例 5: 快速失败（Context 立即取消）")
	ctx5, cancel5 := context.WithCancel(context.Background())
	cancel5() // 立即取消

	err = cli.PublishWithContext(ctx5, "fast.fail", []byte("不会被发送"))
	if errors.Is(err, context.Canceled) {
		fmt.Println("  ✅ 发布被 Context 取消阻止")
	} else {
		log.Printf("  ⚠️  期望取消错误，但得到: %v", err)
	}
	fmt.Println()

	// 示例 6: 查看统计信息
	fmt.Println("示例 6: 客户端统计信息")
	stats := cli.GetStats()
	fmt.Printf("  活跃订阅: %d\n", stats.ActiveSubscriptions)
	fmt.Println()

	// 示例 7: 优雅关闭
	fmt.Println("示例 7: 优雅关闭")
	fmt.Println("  正在关闭连接...")
	cli.Close()
	fmt.Println("  ✅ 连接已关闭，所有 Context 已取消")

	fmt.Println("\n=== 所有示例运行完成 ===")
}
