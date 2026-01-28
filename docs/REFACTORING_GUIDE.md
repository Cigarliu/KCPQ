# KCPQ 重构指南

> 版本: v1.0
> 日期: 2025-01-27
> 分支: feature/usability-improvements
> 目标: 提升易用性评分从 5.3/10 到 8.5/10

## 📋 目录

1. [重构策略](#重构策略)
2. [Phase 1: Context 支持](#phase-1-context-支持)
3. [Phase 2: 增强统计信息](#phase-2-增强统计信息)
4. [Phase 3: 测试与验证](#phase-3-测试与验证)
5. [向后兼容性](#向后兼容性)
6. [测试策略](#测试策略)
7. [迁移指南](#迁移指南)

---

## 🎯 重构策略

### 方案C: 快速迭代（已选定）

**核心原则**：
- ✅ 优先修复 P0 问题（Context、统计）
- ✅ 确保向后兼容
- ✅ 完整的测试覆盖
- ✅ 小步快跑，快速迭代

**迭代周期**：
- Phase 1: Context 支持（Week 1）
- Phase 2: 增强统计（Week 2）
- Phase 3: 测试验证（Week 3）

**延期到 v2.0**：
- ❌ 自动重连机制（复杂度高，需要 120+ 行代码）
- ❌ Channel 订阅模式（需要重新设计 Subscription）

---

## 📝 Phase 1: Context 支持

### 目标

为所有核心方法添加 `context.Context` 支持，符合 2025 年 Go 标准实践。

### API 变更

#### 1. Connect() 方法

**当前实现**（[client.go:47](../client/client.go#L47)）：
```go
func Connect(addr string) (*Client, error) {
    conn, err := kcp.DialWithOptions(addr, nil, 10, 3)
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }
    // ...
}
```

**新实现**：
```go
// ConnectWithContext 带上下文的连接（推荐使用）
func ConnectWithContext(ctx context.Context, addr string) (*Client, error) {
    // 检查 context 是否已取消
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // 设置连接超时
    if deadline, ok := ctx.Deadline(); ok {
        // 使用自定义拨号器支持超时
        // ...
    }

    conn, err := kcp.DialWithOptions(addr, nil, 10, 3)
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }
    // ...
}

// Connect 保持向后兼容（内部调用 ConnectWithContext）
func Connect(addr string) (*Client, error) {
    return ConnectWithContext(context.Background(), addr)
}
```

**使用示例**：
```go
// 带超时连接
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

cli, err := client.ConnectWithContext(ctx, "localhost:4000")
if err != nil {
    log.Fatalf("Connection timeout: %v", err)
}

// 可取消连接
ctx, cancel := context.WithCancel(context.Background())
go func() {
    // 在某些条件下取消连接
    time.Sleep(2 * time.Second)
    cancel()
}()

cli, err := client.ConnectWithContext(ctx, "localhost:4000")
```

#### 2. Subscribe() 方法

**当前实现**（[client.go:192](../client/client.go#L192)）：
```go
func (c *Client) Subscribe(subject string, callback MessageHandler) (*Subscription, error) {
    return c.SubscribeWithOptions(subject, callback, 100)
}
```

**新实现**：
```go
// SubscribeWithContext 带上下文的订阅（推荐使用）
func (c *Client) SubscribeWithContext(
    ctx context.Context,
    subject string,
    callback MessageHandler,
) (*Subscription, error) {
    // 检查 context 是否已取消
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // 等待 ACK 时支持 context 取消
    // ...
}

// Subscribe 保持向后兼容
func (c *Client) Subscribe(subject string, callback MessageHandler) (*Subscription, error) {
    return c.SubscribeWithContext(context.Background(), subject, callback)
}
```

**使用示例**：
```go
// 带超时订阅
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

sub, err := cli.SubscribeWithContext(ctx, "foo.*", func(msg *client.Message) {
    fmt.Printf("Received: %s\n", msg.Data)
})
if err != nil {
    log.Fatalf("Subscription timeout: %v", err)
}
```

#### 3. Publish() 方法

**当前实现**（[client.go:293](../client/client.go#L293)）：
```go
func (c *Client) Publish(subject string, data []byte) error {
    msg := protocol.NewMessageCmd(protocol.CmdPub, subject, data)
    encoded := msg.Encode()
    _, err := c.conn.Write(encoded)
    if err != nil {
        return fmt.Errorf("failed to publish: %w", err)
    }
    return nil
}
```

**新实现**：
```go
// PublishWithContext 带上下文的发布（推荐使用）
func (c *Client) PublishWithContext(
    ctx context.Context,
    subject string,
    data []byte,
) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    msg := protocol.NewMessageCmd(protocol.CmdPub, subject, data)
    encoded := msg.Encode()

    // 支持写入超时
    if deadline, ok := ctx.Deadline(); ok {
        c.conn.SetWriteDeadline(deadline)
        defer c.conn.SetWriteDeadline(time.Time{})
    }

    _, err := c.conn.Write(encoded)
    if err != nil {
        return fmt.Errorf("failed to publish: %w", err)
    }
    return nil
}

// Publish 保持向后兼容
func (c *Client) Publish(subject string, data []byte) error {
    return c.PublishWithContext(context.Background(), subject, data)
}
```

**使用示例**：
```go
// 带超时发布
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
defer cancel()

err := cli.PublishWithContext(ctx, "foo.bar", []byte("Hello"))
if errors.Is(err, context.DeadlineExceeded) {
    log.Printf("Publish timeout")
}
```

### 实现细节

#### Context 传递到 receiveLoop

在 [receiveLoop()](../client/client.go#L83) 中需要监听 context：

```go
func (c *Client) receiveLoop() {
    defer close(c.receiveLoopDone)

    for {
        select {
        case <-c.ctx.Done():
            log.Printf("[INFO] receiveLoop stopped by context")
            return
        default:
            c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))

            msg, err := protocol.ParseFromReader(c.conn)
            if err != nil {
                // ...
            }

            c.handleMessage(msg)
        }
    }
}
```

#### 修改 Client 结构体

```go
type Client struct {
    conn               *kcp.UDPSession
    ctx                context.Context    // 新增：用于取消操作
    cancel             context.CancelFunc // 新增：取消函数
    subscriptions      []*Subscription
    mu                 sync.RWMutex
    done               chan struct{}
    receiveLoopDone    chan struct{}
    heartbeatDone      chan struct{}
    onConnectionLost   ConnectionLostCallback
    pendingSubs        map[string]*subscriptionACK
    pendingSubsMu      sync.RWMutex
}
```

### 优先级

🔴 **P0 - 必须实现**
- ConnectWithContext()
- SubscribeWithContext()
- PublishWithContext()
- 向后兼容的 Connect()、Subscribe()、Publish()

---

## 📊 Phase 2: 增强统计信息

### 目标

扩展 `ClientStats` 结构体，提供丰富的可观测性指标。

### 当前实现

**ClientStats**（[client.go:363](../client/client.go#L363)）：
```go
type ClientStats struct {
    ActiveSubscriptions int
}
```

### 新实现

```go
// ClientStats 客户端统计信息（增强版）
type ClientStats struct {
    // 连接状态
    Connected            bool
    ConnectedAt          time.Time
    DisconnectedAt       time.Time

    // 消息统计
    MessagesSent         int64
    MessagesReceived     int64
    MessagesSentPerSec   float64
    MessagesReceivedPerSec float64

    // 网络统计
    BytesSent            int64
    BytesReceived        int64
    AvgLatency           time.Duration
    LastLatency          time.Duration

    // 订阅统计
    ActiveSubscriptions  int
    TotalSubscriptions   int64

    // 错误统计
    ConnectionErrors     int64
    PublishErrors        int64
    SubscriptionErrors   int64

    // 重连统计（为 v2.0 准备）
    ReconnectCount       int64
    LastReconnectedAt    time.Time
}
```

### 实现细节

#### 1. 添加原子计数器

在 Client 结构体中添加统计字段：

```go
type Client struct {
    // ... 现有字段

    // 统计字段（使用原子操作）
    messagesSent       atomic.Int64
    messagesReceived   atomic.Int64
    bytesSent          atomic.Int64
    bytesReceived      atomic.Int64
    connectionErrors   atomic.Int64
    publishErrors      atomic.Int64
    subscriptionErrors atomic.Int64

    // 延迟统计（需要互斥锁保护）
    latencyMu          sync.RWMutex
    latencies          []time.Duration // 滑动窗口
    lastLatency        time.Duration

    // 连接时间
    connectedAt        time.Time
}
```

#### 2. 在关键路径更新统计

**Publish() 方法**：
```go
func (c *Client) PublishWithContext(ctx context.Context, subject string, data []byte) error {
    start := time.Now()
    defer func() {
        // 记录延迟
        latency := time.Since(start)
        c.recordLatency(latency)

        // 记录字节数
        c.bytesSent.Add(int64(len(data)))
    }()

    // ... 发布逻辑

    if err != nil {
        c.publishErrors.Add(1)
        return err
    }

    c.messagesSent.Add(1)
    return nil
}
```

**receiveLoop() 方法**：
```go
func (c *Client) receiveLoop() {
    defer close(c.receiveLoopDone)

    for {
        select {
        case <-c.done:
            return
        default:
            msg, err := protocol.ParseFromReader(c.conn)
            if err != nil {
                c.connectionErrors.Add(1)
                return
            }

            c.messagesReceived.Add(1)
            c.bytesReceived.Add(int64(len(msg.Payload)))

            c.handleMessage(msg)
        }
    }
}
```

#### 3. 计算 QPS 和延迟

```go
func (c *Client) GetStats() ClientStats {
    c.mu.RLock()
    defer c.mu.RUnlock()

    // 计算运行时间
    var uptime time.Duration
    if !c.connectedAt.IsZero() {
        uptime = time.Since(c.connectedAt)
    }

    // 计算 QPS
    var sentPerSec, receivedPerSec float64
    if uptime.Seconds() > 0 {
        sentPerSec = float64(c.messagesSent.Load()) / uptime.Seconds()
        receivedPerSec = float64(c.messagesReceived.Load()) / uptime.Seconds()
    }

    // 计算平均延迟
    avgLatency := c.calculateAvgLatency()

    return ClientStats{
        Connected:            c.conn != nil,
        ConnectedAt:          c.connectedAt,
        MessagesSent:         c.messagesSent.Load(),
        MessagesReceived:     c.messagesReceived.Load(),
        MessagesSentPerSec:   sentPerSec,
        MessagesReceivedPerSec: receivedPerSec,
        BytesSent:            c.bytesSent.Load(),
        BytesReceived:        c.bytesReceived.Load(),
        AvgLatency:           avgLatency,
        LastLatency:          c.lastLatency,
        ActiveSubscriptions:  c.countActiveSubscriptions(),
        ConnectionErrors:     c.connectionErrors.Load(),
        PublishErrors:        c.publishErrors.Load(),
        SubscriptionErrors:   c.subscriptionErrors.Load(),
    }
}

func (c *Client) recordLatency(latency time.Duration) {
    c.latencyMu.Lock()
    defer c.latencyMu.Unlock()

    c.lastLatency = latency

    // 滑动窗口：保留最近 1000 个样本
    c.latencies = append(c.latencies, latency)
    if len(c.latencies) > 1000 {
        c.latencies = c.latencies[1:]
    }
}

func (c *Client) calculateAvgLatency() time.Duration {
    c.latencyMu.Lock()
    defer c.latencyMu.Unlock()

    if len(c.latencies) == 0 {
        return 0
    }

    var sum time.Duration
    for _, l := range c.latencies {
        sum += l
    }

    return sum / time.Duration(len(c.latencies))
}
```

### 使用示例

```go
// 获取统计信息
stats := cli.GetStats()
fmt.Printf("Connected: %v\n", stats.Connected)
fmt.Printf("Messages Sent: %d (%.2f msg/s)\n", stats.MessagesSent, stats.MessagesSentPerSec)
fmt.Printf("Messages Received: %d (%.2f msg/s)\n", stats.MessagesReceived, stats.MessagesReceivedPerSec)
fmt.Printf("Avg Latency: %v\n", stats.AvgLatency)
fmt.Printf("Errors: %d connections, %d publishes\n", stats.ConnectionErrors, stats.PublishErrors)
```

### 优先级

🟡 **P1 - 严重影响体验**
- 扩展 ClientStats 结构体
- 添加原子计数器
- 实现 QPS 计算
- 实现延迟统计

---

## ✅ Phase 3: 测试与验证

### 目标

确保所有新功能都有完整的测试覆盖，并且向后兼容。

### 单元测试

创建 `client/client_context_test.go`：

```go
package client

import (
    "context"
    "errors"
    "testing"
    "time"
)

// TestConnectWithContext 测试带 Context 的连接
func TestConnectWithContext(t *testing.T) {
    tests := []struct {
        name      string
        ctx       context.Context
        wantErr   error
    }{
        {
            name:    "正常连接",
            ctx:     context.Background(),
            wantErr: nil,
        },
        {
            name:    "已取消的 Context",
            ctx:     func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }(),
            wantErr: context.Canceled,
        },
        {
            name:    "超时 Context",
            ctx:     func() context.Context { ctx, _ := context.WithTimeout(context.Background(), 1*time.Nanosecond); time.Sleep(10 * time.Millisecond); return ctx }(),
            wantErr: context.DeadlineExceeded,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cli, err := ConnectWithContext(tt.ctx, "localhost:4000")

            if tt.wantErr != nil {
                if err == nil {
                    t.Errorf("期望错误 %v，但得到 nil", tt.wantErr)
                } else if !errors.Is(err, tt.wantErr) {
                    t.Errorf("期望错误 %v，但得到 %v", tt.wantErr, err)
                }
                return
            }

            if err != nil {
                t.Fatalf("不期望错误: %v", err)
            }

            if cli == nil {
                t.Fatal("期望客户端，但得到 nil")
            }

            defer cli.Close()
        })
    }
}

// TestSubscribeWithContext 测试带 Context 的订阅
func TestSubscribeWithContext(t *testing.T) {
    // 启动测试服务器
    // ...

    cli, err := Connect("localhost:4000")
    if err != nil {
        t.Fatal(err)
    }
    defer cli.Close()

    tests := []struct {
        name    string
        ctx     context.Context
        subject string
        wantErr error
    }{
        {
            name:    "正常订阅",
            ctx:     context.Background(),
            subject: "test.*",
            wantErr: nil,
        },
        {
            name:    "已取消的 Context",
            ctx:     func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }(),
            subject: "test.*",
            wantErr: context.Canceled,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            sub, err := cli.SubscribeWithContext(tt.ctx, tt.subject, func(msg *Message) {
                // 处理消息
            })

            if tt.wantErr != nil {
                if err == nil {
                    t.Errorf("期望错误 %v，但得到 nil", tt.wantErr)
                } else if !errors.Is(err, tt.wantErr) {
                    t.Errorf("期望错误 %v，但得到 %v", tt.wantErr, err)
                }
                return
            }

            if err != nil {
                t.Fatalf("不期望错误: %v", err)
            }

            if sub == nil {
                t.Fatal("期望订阅，但得到 nil")
            }

            defer sub.Unsubscribe()
        })
    }
}

// TestPublishWithContext 测试带 Context 的发布
func TestPublishWithContext(t *testing.T) {
    // ...
}

// TestClientStats 测试统计信息
func TestClientStats(t *testing.T) {
    // 启动测试服务器
    // ...

    cli, err := Connect("localhost:4000")
    if err != nil {
        t.Fatal(err)
    }
    defer cli.Close()

    // 发布一些消息
    for i := 0; i < 100; i++ {
        err := cli.Publish("test", []byte("hello"))
        if err != nil {
            t.Fatal(err)
        }
    }

    // 获取统计
    stats := cli.GetStats()

    if stats.MessagesSent != 100 {
        t.Errorf("期望 100 条消息，但得到 %d", stats.MessagesSent)
    }

    if stats.MessagesSentPerSec <= 0 {
        t.Errorf("期望正的 QPS，但得到 %f", stats.MessagesSentPerSec)
    }

    if !stats.Connected {
        t.Error("期望已连接状态")
    }
}

// TestBackwardCompatibility 测试向后兼容性
func TestBackwardCompatibility(t *testing.T) {
    // 测试旧 API 仍然工作
    cli, err := Connect("localhost:4000")
    if err != nil {
        t.Fatal(err)
    }
    defer cli.Close()

    sub, err := cli.Subscribe("test.*", func(msg *Message) {
        // 处理消息
    })
    if err != nil {
        t.Fatal(err)
    }
    defer sub.Unsubscribe()

    err = cli.Publish("test.hello", []byte("world"))
    if err != nil {
        t.Fatal(err)
    }

    // 应该仍然工作
    stats := cli.GetStats()
    if !stats.Connected {
        t.Error("期望已连接状态")
    }
}
```

### 功能测试

创建 `examples/context_usage/main.go`：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/kcpq/client"
)

func main() {
    // 示例 1: 带超时的连接
    fmt.Println("=== 示例 1: 带超时的连接 ===")
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    cli, err := client.ConnectWithContext(ctx, "localhost:4000")
    if err != nil {
        log.Fatalf("连接失败: %v", err)
    }
    defer cli.Close()
    fmt.Printf("✅ 连接成功\n")

    // 示例 2: 带超时的订阅
    fmt.Println("\n=== 示例 2: 带超时的订阅 ===")
    ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel2()

    sub, err := cli.SubscribeWithContext(ctx2, "demo.*", func(msg *client.Message) {
        fmt.Printf("收到消息: %s = %s\n", msg.Subject, string(msg.Data))
    })
    if err != nil {
        log.Fatalf("订阅失败: %v", err)
    }
    defer sub.Unsubscribe()
    fmt.Printf("✅ 订阅成功\n")

    // 示例 3: 查看统计信息
    fmt.Println("\n=== 示例 3: 统计信息 ===")

    // 发布一些消息
    for i := 0; i < 10; i++ {
        err := cli.Publish("demo.hello", []byte(fmt.Sprintf("消息 %d", i)))
        if err != nil {
            log.Printf("发布失败: %v", err)
        }
        time.Sleep(100 * time.Millisecond)
    }

    time.Sleep(1 * time.Second)

    stats := cli.GetStats()
    fmt.Printf("已连接: %v\n", stats.Connected)
    fmt.Printf("发送消息: %d (%.2f msg/s)\n", stats.MessagesSent, stats.MessagesSentPerSec)
    fmt.Printf("接收消息: %d (%.2f msg/s)\n", stats.MessagesReceived, stats.MessagesReceivedPerSec)
    fmt.Printf("平均延迟: %v\n", stats.AvgLatency)
    fmt.Printf("活跃订阅: %d\n", stats.ActiveSubscriptions)

    fmt.Println("\n✅ 所有示例运行完成")
}
```

### 运行测试

```bash
# 单元测试
cd client
go test -v -race -cover

# 功能测试
cd examples/context_usage
go run main.go

# 集成测试（使用真实的 H.264 视频流）
cd examples/h264_test
go test -v
```

### 成功标准

✅ **必须满足**：
1. 所有单元测试通过
2. 测试覆盖率 > 80%
3. 无 data race 警告
4. 向后兼容性测试通过
5. H.264 视频流测试达到 200+ fps
6. 内存泄漏检测通过

---

## 🔄 向后兼容性

### 策略

**完全向后兼容**：所有旧代码无需修改即可继续工作。

### 实现方式

1. **保留旧 API**
   ```go
   // 旧 API（保留）
   func Connect(addr string) (*Client, error) {
       return ConnectWithContext(context.Background(), addr)
   }
   ```

2. **添加新 API**
   ```go
   // 新 API（推荐使用）
   func ConnectWithContext(ctx context.Context, addr string) (*Client, error) {
       // 新实现
   }
   ```

3. **文档标注**
   ```go
   // Connect 连接到服务器（已废弃：建议使用 ConnectWithContext）
   // Deprecated: 使用 ConnectWithContext 以支持 context.Context
   func Connect(addr string) (*Client, error) {
       return ConnectWithContext(context.Background(), addr)
   }
   ```

### 迁移路径

**渐进式迁移**：
- **第一阶段**：旧代码继续使用旧 API
- **第二阶段**：新代码使用新 API
- **第三阶段**：逐步迁移旧代码
- **第四阶段**（v3.0）：正式废弃旧 API

---

## 🧪 测试策略

### 测试金字塔

```
        /\
       /  \
      / E2E \      <- 5% (H.264 视频流测试)
     /--------\
    /  集成测试  \   <- 15% (服务器-客户端集成)
   /--------------\
  /    单元测试     \ <- 80% (Context、统计功能)
 /--------------------\
```

### 测试覆盖

| 组件 | 测试文件 | 覆盖目标 |
|------|---------|---------|
| ConnectWithContext | client_context_test.go | 90% |
| SubscribeWithContext | client_context_test.go | 85% |
| PublishWithContext | client_context_test.go | 85% |
| ClientStats | client_stats_test.go | 90% |
| 向后兼容性 | client_compat_test.go | 100% |

### 性能基准

在 `benchmark_test.go` 中添加：

```go
// BenchmarkConnectWithContext 性能基准测试
func BenchmarkConnectWithContext(b *testing.B) {
    // ...
}

// BenchmarkPublishWithContext 性能基准测试
func BenchmarkPublishWithContext(b *testing.B) {
    // ...
}

// BenchmarkClientStats 性能基准测试
func BenchmarkClientStats(b *testing.B) {
    // ...
}
```

### 压力测试

```bash
# 并发连接测试
go test -v -race -run=TestConcurrentConnect

# 高吞吐量测试
go test -bench=BenchmarkPublishThroughput -benchmem

# 长时间稳定性测试
go test -v -run=TestLongRunningStability -timeout 1h
```

---

## 📚 迁移指南

### 代码迁移

#### 旧代码（v1.0）

```go
cli, err := client.Connect("localhost:4000")
if err != nil {
    log.Fatal(err)
}
defer cli.Close()

sub, err := cli.Subscribe("foo.*", func(msg *client.Message) {
    fmt.Printf("Received: %s\n", msg.Data)
})
if err != nil {
    log.Fatal(err)
}

err = cli.Publish("foo.bar", []byte("Hello"))
```

#### 新代码（v1.5 - 推荐使用）

```go
// 带超时的连接
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

cli, err := client.ConnectWithContext(ctx, "localhost:4000")
if err != nil {
    log.Fatal(err)
}
defer cli.Close()

// 带超时的订阅
ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel2()

sub, err := cli.SubscribeWithContext(ctx2, "foo.*", func(msg *client.Message) {
    fmt.Printf("Received: %s\n", msg.Data)
})
if err != nil {
    log.Fatal(err)
}

// 带超时的发布
ctx3, cancel3 := context.WithTimeout(context.Background(), 1*time.Second)
defer cancel3()

err = cli.PublishWithContext(ctx3, "foo.bar", []byte("Hello"))

// 查看统计信息
stats := cli.GetStats()
fmt.Printf("Sent: %d msg/s, Avg Latency: %v\n",
    stats.MessagesSentPerSec, stats.AvgLatency)
```

### API 对比表

| 旧 API | 新 API | 优势 |
|--------|--------|------|
| `Connect(addr)` | `ConnectWithContext(ctx, addr)` | 支持超时、取消 |
| `Subscribe(subject, cb)` | `SubscribeWithContext(ctx, subject, cb)` | 支持超时、取消 |
| `Publish(subject, data)` | `PublishWithContext(ctx, subject, data)` | 支持超时、取消 |
| `GetStats()` | `GetStats()` | 更丰富的统计信息 |

### 迁移检查清单

- [ ] 更新连接代码，添加 Context
- [ ] 更新订阅代码，添加 Context
- [ ] 更新发布代码，添加 Context
- [ ] 添加统计信息监控
- [ ] 更新错误处理（context.Canceled, context.DeadlineExceeded）
- [ ] 运行单元测试
- [ ] 运行功能测试
- [ ] 性能基准测试
- [ ] 压力测试

---

## 📅 实施时间表

### Week 1: Context 支持

**Day 1-2**: API 设计
- [ ] 定义新 API 签名
- [ ] 设计 Context 传递机制
- [ ] 编写设计文档

**Day 3-4**: 实现
- [ ] 实现 ConnectWithContext()
- [ ] 实现 SubscribeWithContext()
- [ ] 实现 PublishWithContext()
- [ ] 修改 receiveLoop() 支持 Context

**Day 5**: 测试
- [ ] 编写单元测试
- [ ] 编写功能测试
- [ ] 向后兼容性测试

### Week 2: 增强统计

**Day 1-2**: 设计
- [ ] 定义 ClientStats 结构体
- [ ] 设计统计收集机制
- [ ] 设计性能优化方案

**Day 3-4**: 实现
- [ ] 添加原子计数器
- [ ] 实现 QPS 计算
- [ ] 实现延迟统计
- [ ] 实现错误统计

**Day 5**: 测试
- [ ] 编写统计测试
- [ ] 性能基准测试
- [ ] 内存泄漏检测

### Week 3: 测试与验证

**Day 1-2**: 集成测试
- [ ] 端到端测试
- [ ] H.264 视频流测试
- [ ] 并发压力测试

**Day 3-4**: 文档
- [ ] 更新 API 文档
- [ ] 编写迁移指南
- [ ] 更新 README

**Day 5**: 发布准备
- [ ] 代码审查
- [ ] 最终测试
- [ ] 合并到主分支
- [ ] 打 tag v1.5

---

## 🎯 预期成果

### 易用性提升

| 维度 | v1.0 评分 | v1.5 目标 | 提升 |
|------|-----------|-----------|------|
| Context 支持 | 0/10 | 9/10 | +9 |
| 统计监控 | 4/10 | 8/10 | +4 |
| API 设计 | 6/10 | 8/10 | +2 |
| 生产就绪 | 5/10 | 8/10 | +3 |
| **总体评分** | **5.3/10** | **8.5/10** | **+3.2** |

### 性能指标

- ✅ 保持 H.264 视频流 205+ fps
- ✅ 延迟 < 10ms
- ✅ 统计开销 < 1% CPU
- ✅ 内存开销 < 5MB

### 质量指标

- ✅ 测试覆盖率 > 80%
- ✅ 无 data race
- ✅ 无内存泄漏
- ✅ 100% 向后兼容

---

## 📖 参考资料

### Context 最佳实践

- [Go Context 官方文档](https://golang.org/pkg/context/)
- [Context 使用指南](https://go.dev/blog/context)

### NATS API 设计

- [NATS Go Client](https://github.com/nats-io/nats.go)
- [NATS 连接管理](https://docs.nats.io/developing-with-nats/connecting)

### 统计信息设计

- [Prometheus 指标最佳实践](https://prometheus.io/docs/practices/naming/)
- [Go 性能优化实践](https://go.dev/doc/diagnostics)

---

**文档版本**: v1.0
**最后更新**: 2025-01-27
**维护者**: KCPQ Team
