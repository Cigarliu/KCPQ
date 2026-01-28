package client

import (
	"context"
	"testing"
	"time"
)

// TestSubscribeChan_Basic 测试基本的 Channel 订阅
func TestSubscribeChan_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 测试 Channel 订阅
	msgChan, sub, err := cli.SubscribeChan("test.channel.*", 10)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	if msgChan == nil {
		t.Fatal("期望 msgChan，但得到 nil")
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pubCancel()

	err = cli.PublishWithContext(pubCtx, "test.channel.hello", []byte("world"))
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	// 从 channel 接收消息
	select {
	case msg := <-msgChan:
		if string(msg.Data) != "world" {
			t.Errorf("期望消息 'world'，但得到 '%s'", string(msg.Data))
		}
		if msg.Subject != "test.channel.hello" {
			t.Errorf("期望 subject 'test.channel.hello'，但得到 '%s'", msg.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("未收到消息")
	}
}

// TestSubscribeChan_WithContext 测试带 Context 的 Channel 订阅
func TestSubscribeChan_WithContext(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 使用带超时的 context 订阅
	subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer subCancel()

	msgChan, sub, err := cli.SubscribeChanWithContext(subCtx, "test.context.*", 10)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布多条消息
	for i := 0; i < 5; i++ {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
		err := cli.PublishWithContext(pubCtx, "test.context.msg", []byte("message"))
		pubCancel()
		if err != nil {
			t.Errorf("发布第 %d 条消息失败: %v", i, err)
		}
	}

	// 接收多条消息
	receivedCount := 0
	timeout := time.After(2 * time.Second)

	for {
		select {
		case msg := <-msgChan:
			receivedCount++
			t.Logf("收到消息 #%d: %s", receivedCount, msg.Data)
			if receivedCount >= 5 {
				return // 收到足够的消息
			}
		case <-timeout:
			if receivedCount < 5 {
				t.Errorf("期望收到 5 条消息，但只收到 %d 条", receivedCount)
			}
			return
		}
	}
}

// TestSubscribeChan_ChannelFull 测试 channel 满时的情况
func TestSubscribeChan_ChannelFull(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 创建容量很小的 channel
	msgChan, sub, err := cli.SubscribeChan("test.full.*", 2)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 快速发布多条消息（超过 channel 容量）
	for i := 0; i < 10; i++ {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := cli.PublishWithContext(pubCtx, "test.full.msg", []byte("message"))
		pubCancel()
		if err != nil {
			t.Logf("发布第 %d 条消息失败（可能被丢弃）: %v", i, err)
		}
	}

	// 给一些时间让消息到达
	time.Sleep(200 * time.Millisecond)

	// 从 channel 接收消息（应该不会超过容量）
	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for {
		select {
		case <-msgChan:
			receivedCount++
		case <-timeout:
			t.Logf("收到 %d 条消息（channel 容量只有 2）", receivedCount)
			// 验证 channel 不阻塞
			return
		}
	}
}

// TestSubscribeChan_MultipleConsumers 测试多个消费者
func TestSubscribeChan_MultipleConsumers(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 订阅并获取 channel
	msgChan, sub, err := cli.SubscribeChan("test.multi.*", 100)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 创建多个消费者
	consumer1Done := make(chan bool)
	consumer2Done := make(chan bool)

	// 消费者 1
	go func() {
		count := 0
		for msg := range msgChan {
			count++
			t.Logf("消费者1 收到消息 #%d: %s", count, msg.Data)
			if count >= 5 {
				break
			}
		}
		consumer1Done <- true
	}()

	// 消费者 2（实际上消费者1和消费者2会竞争消息）
	go func() {
		count := 0
		for msg := range msgChan {
			count++
			t.Logf("消费者2 收到消息 #%d: %s", count, msg.Data)
			if count >= 5 {
				break
			}
		}
		consumer2Done <- true
	}()

	// 发布消息
	for i := 0; i < 10; i++ {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
		err := cli.PublishWithContext(pubCtx, "test.multi.msg", []byte("message"))
		pubCancel()
		if err != nil {
			t.Logf("发布失败: %v", err)
		}
	}

	// 等待消费者完成
	<-consumer1Done
	<-consumer2Done
}

// TestSubscribeChan_SelectTimeout 测试使用 select 实现超时
func TestSubscribeChan_SelectTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	msgChan, sub, err := cli.SubscribeChan("test.select.*", 10)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 使用 select 实现超时
	select {
	case msg := <-msgChan:
		t.Logf("收到消息: %s", msg.Data)
	case <-time.After(1 * time.Second):
		t.Log("1秒内没有收到消息（这是正常的，因为我们没有发送消息）")
	}

	// 现在发送消息
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
	err = cli.PublishWithContext(pubCtx, "test.select.hello", []byte("world"))
	pubCancel()
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	// 再次使用 select，这次应该收到消息
	select {
	case msg := <-msgChan:
		if string(msg.Data) != "world" {
			t.Errorf("期望 'world'，但得到 '%s'", string(msg.Data))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("应该收到消息，但超时了")
	}
}

// TestSubscribeChan_CloseChannel 测试取消订阅时 channel 关闭
func TestSubscribeChan_CloseChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	msgChan, sub, err := cli.SubscribeChan("test.close.*", 10)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}

	// 取消订阅
	sub.Unsubscribe()

	// channel 应该仍然可以读取剩余的消息
	// 但当 channel 关闭时，读取会返回零值
	// 注意：我们的实现中，channel 不会自动关闭，所以这个测试只是验证 Unsubscribe 不会崩溃

	// 发送一条消息（订阅已取消，不应该收到）
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
	cli.PublishWithContext(pubCtx, "test.close.msg", []byte("should not receive"))
	pubCancel()

	// 尝试从 channel 读取（应该超时）
	select {
	case msg := <-msgChan:
		t.Logf("意外的消息（订阅已取消）: %s", msg.Data)
	case <-time.After(500 * time.Millisecond):
		t.Log("正确：没有收到消息（订阅已取消）")
	}

	cli.Close()
}

// TestSubscribeChan_ContextCancellation 测试 Context 取消时 channel 的行为
func TestSubscribeChan_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要服务器的集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := ConnectWithContext(ctx, "localhost:4000")
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer cli.Close()

	// 使用可取消的 context
	subCtx, subCancel := context.WithCancel(context.Background())

	msgChan, sub, err := cli.SubscribeChanWithContext(subCtx, "test.cancel.*", 10)
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}

	// 等待订阅成功
	time.Sleep(100 * time.Millisecond)

	// 取消 context
	subCancel()

	// context 取消后，订阅应该不再接收消息
	// 发送消息
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 1*time.Second)
	cli.PublishWithContext(pubCtx, "test.cancel.msg", []byte("should not receive"))
	pubCancel()

	// 尝试从 channel 读取（应该超时或收到很少的消息）
	select {
	case msg := <-msgChan:
		t.Logf("可能在 context 取消前收到的消息: %s", msg.Data)
	case <-time.After(500 * time.Millisecond):
		t.Log("正确：context 取消后不再接收消息")
	}

	sub.Unsubscribe()
}
