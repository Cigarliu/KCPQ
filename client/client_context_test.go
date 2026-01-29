package client

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestConnectWithContext_Basic 测试带 Context 的正常连接
func TestConnectWithContext_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	s, key := startTestServer(t)

	ctx := context.Background()
	cli, err := ConnectWithContext(ctx, s.Addr().String(), key)

	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	if cli == nil {
		t.Fatal("期望客户端，但得到 nil")
	}

	// 验证 context 已正确设置
	if cli.ctx == nil {
		t.Error("context 未初始化")
	}

	if cli.cancel == nil {
		t.Error("cancel 函数未初始化")
	}

	// 清理
	cli.Close()
}

// TestConnectWithContext_Cancelled 测试已取消的 Context
func TestConnectWithContext_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	cli, err := ConnectWithContext(ctx, "127.0.0.1:1", testAES256Key())

	if err == nil {
		if cli != nil {
			cli.Close()
		}
		t.Fatal("期望错误，但得到 nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled，但得到 %v", err)
	}
}

// TestSubscribeWithContext 测试带 Context 的订阅
func TestSubscribeWithContext(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	// 连接服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s, key := startTestServer(t)
	cli, err := ConnectWithContext(ctx, s.Addr().String(), key)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 测试正常订阅
	subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer subCancel()

	received := make(chan *Message, 1)

	sub, err := cli.SubscribeWithContext(subCtx, "test.context.*", func(msg *Message) {
		select {
		case received <- msg:
		default:
		}
	})

	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}

	if sub == nil {
		t.Fatal("期望订阅，但得到 nil")
	}

	defer sub.Unsubscribe()

	// 等待一下确保订阅成功
	time.Sleep(100 * time.Millisecond)

	// 发送测试消息
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pubCancel()

	err = cli.PublishWithContext(pubCtx, "test.context.hello", []byte("world"))
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	// 等待接收消息
	select {
	case msg := <-received:
		if string(msg.Data) != "world" {
			t.Errorf("期望消息 'world'，但得到 '%s'", string(msg.Data))
		}
		if msg.Subject != "test.context.hello" {
			t.Errorf("期望 subject 'test.context.hello'，但得到 '%s'", msg.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("未收到消息")
	}
}

// TestPublishWithContext 测试带 Context 的发布
func TestPublishWithContext(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	// 连接服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s, key := startTestServer(t)
	cli, err := ConnectWithContext(ctx, s.Addr().String(), key)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 测试正常发布
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pubCancel()

	err = cli.PublishWithContext(pubCtx, "test.publish.hello", []byte("world"))
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	// 发布多条消息
	for i := 0; i < 10; i++ {
		pubCtx2, pubCancel2 := context.WithTimeout(context.Background(), 1*time.Second)
		err = cli.PublishWithContext(pubCtx2, "test.publish.multiple", []byte("message"))
		pubCancel2()
		if err != nil {
			t.Errorf("发布第 %d 条消息失败: %v", i, err)
		}
	}
}

// TestBackwardCompatibility 测试向后兼容性
func TestBackwardCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	s, key := startTestServer(t)

	// 测试旧 API（不带 context）
	cli, err := Connect(s.Addr().String(), key)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 验证 context 已自动设置
	if cli.ctx == nil {
		t.Error("context 未自动初始化")
	}

	if cli.cancel == nil {
		t.Error("cancel 函数未自动初始化")
	}

	// 测试旧 API 订阅
	received := make(chan *Message, 1)
	sub, err := cli.Subscribe("test.compat.*", func(msg *Message) {
		select {
		case received <- msg:
		default:
		}
	})

	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}

	if sub == nil {
		t.Fatal("期望订阅，但得到 nil")
	}

	defer sub.Unsubscribe()

	// 等待订阅成功
	time.Sleep(100 * time.Millisecond)

	// 测试旧 API 发布
	err = cli.Publish("test.compat.hello", []byte("world"))
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	// 等待接收消息
	select {
	case msg := <-received:
		if string(msg.Data) != "world" {
			t.Errorf("期望消息 'world'，但得到 '%s'", string(msg.Data))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("未收到消息")
	}

	// 测试 GetStats（暂时只检查 ActiveSubscriptions，Phase 2 会添加 Connected 字段）
	stats := cli.GetStats()
	if stats.ActiveSubscriptions != 1 {
		t.Errorf("期望 1 个活跃订阅，但得到 %d", stats.ActiveSubscriptions)
	}
}

// TestClientClose_ContextCancellation 测试 Close 时 Context 取消
func TestClientClose_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s, key := startTestServer(t)
	cli, err := ConnectWithContext(ctx, s.Addr().String(), key)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 验证 context 未取消
	select {
	case <-cli.ctx.Done():
		t.Fatal("context 不应该已取消")
	default:
	}

	// 关闭客户端
	cli.Close()

	// 验证 context 已取消
	select {
	case <-cli.ctx.Done():
		// 正常
	case <-time.After(1 * time.Second):
		t.Fatal("context 应该已取消")
	}
}
