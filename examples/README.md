# KCPQ Examples

本目录包含 KCPQ v2.0 的示例应用程序，展示如何在实际场景中使用 KCPQ 客户端库。

## 📁 目录结构

```
examples/
├── server/           # KCPQ 服务器示例
├── h264-relay/       # H.264 视频流转发示例
├── kcpq-to-nats/     # KCPQ 到 NATS 转发示例
├── context_usage/    # Context 使用示例
└── comprehensive_test/ # 综合功能测试示例
```

## 🚀 示例程序

### 1. **server** - KCPQ 服务器

最简单的 KCPQ 服务器实现，用于演示和测试。

```bash
cd server
go run main.go
```

**特性**:
- 监听端口: `4000` (可通过环境变量 `KCP_NATS_ADDR` 配置)
- 支持 Pub/Sub 消息传递
- 内置统计信息显示
- Pprof 性能分析支持（端口 `6060`）

---

### 2. **h264-relay** - H.264 视频流转发

接收本地 H.264 视频流（通过 UDP），然后转发到 KCPQ 服务器。

**使用场景**: 将监控摄像头、视频编码器等设备输出的 H.264 视频流实时传输到远程服务器。

```bash
cd h264-relay
go run main.go
```

**配置** (通过环境变量):
```bash
# KCPQ 服务器地址
export KCPQ_SERVER=localhost:4000

# 发布主题
export KCPQ_SUBJECT=h264.stream

# UDP 监听地址
export UDP_LISTEN=:22345
```

**默认配置**:
- UDP 监听: `:22345`
- KCPQ 服务器: `localhost:4000`
- 发布主题: `h264.stream`
- 缓冲区大小: 10 MB

**KCPQ v2.0 特性展示**:
- ✅ `ConnectWithContext()` - Context 支持
- ✅ `PublishWithContext()` - 带超时的发布
- ✅ `EnableAutoReconnect()` - 自动重连
- ✅ 增强统计信息

**统计信息** (每10秒自动显示):
```
H.264 Relay Statistics
  Frames: 1234, Rate: 25.00 fps
  Data: 15.2 MB, Rate: 1.5 MB/s
  Bitrate: 12.1 Mbps

KCPQ Client Stats (v2.0):
  Connected: true
  Messages Sent: 1234
  Avg Latency: 5ms
  Reconnect Count: 0
```

---

### 3. **kcpq-to-nats** - KCPQ 到 NATS 转发

从 KCPQ 服务器订阅消息，然后转发到 NATS 服务器。

**使用场景**: 将 KCPQ 的低延迟消息桥接到 NATS 生态系统，用于与其他系统集成。

```bash
cd kcpq-to-nats
go run main.go
```

**配置** (通过环境变量):
```bash
# KCPQ 服务器地址
export KCPQ_SERVER=localhost:4000

# KCPQ 订阅主题
export KCPQ_SUBJECT=h264.stream

# NATS 服务器地址
export NATS_SERVER=nats://localhost:4222

# NATS 发布主题
export NATS_SUBJECT=h264.stream
```

**默认配置**:
- KCPQ 服务器: `localhost:4000`
- KCPQ 主题: `h264.stream`
- NATS 服务器: `nats://localhost:4222`
- NATS 主题: `h264.stream`

**KCPQ v2.0 特性展示**:
- ✅ `ConnectWithContext()` - Context 支持
- ✅ `SubscribeChanWithContext()` - **Channel 订阅模式**（推荐）
- ✅ `EnableAutoReconnect()` - 自动重连
- ✅ 增强统计信息

**Channel vs Callback 订阅模式**:
```go
// v2.0 推荐: Channel 订阅模式
msgChan, sub, err := client.SubscribeChanWithContext(
    ctx,
    "h264.stream",
    1000, // channel capacity
)

for msg := range msgChan {
    // 处理消息
}

// v1.x 传统: Callback 订阅模式（向后兼容）
sub, err := client.SubscribeWithContext(
    ctx,
    "h264.stream",
    func(msg *client.Message) {
        // 处理消息
    },
)
```

---

### 4. **context_usage** - Context 使用示例

展示如何在 KCPQ 中使用 Go Context 进行超时控制和取消操作。

```bash
cd context_usage
go run main.go
```

**演示功能**:
- 带超时的连接: `ConnectWithContext(ctx, addr)`
- 带超时的订阅: `SubscribeWithContext(ctx, subject)`
- 带超时的发布: `PublishWithContext(ctx, subject, data)`
- Context 取消时的优雅关闭

---

### 5. **comprehensive_test** - 综合功能测试

完整测试所有 KCPQ v2.0 功能的示例程序。

```bash
cd comprehensive_test
go run main.go
```

**测试内容**:
1. ✅ Context 支持（超时、取消）
2. ✅ Channel 订阅模式
3. ✅ 自动重连功能
4. ✅ 统计信息功能

---

## 🆕 KCPQ v2.0 新特性

### 1. Context 支持

所有核心操作都支持 Context:

```go
// 连接（带超时）
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

client, err := kcpq.ConnectWithContext(ctx, "localhost:4000")

// 订阅（带取消）
sub, err := client.SubscribeWithContext(ctx, "subject", callback)

// 发布（带超时）
err := client.PublishWithContext(ctx, "subject", data)
```

### 2. Channel 订阅模式

更符合 Go 惯用法的订阅方式:

```go
msgChan, sub, err := client.SubscribeChan(ctx, "subject", 1000)

// 使用 select 处理多个 channel
select {
case msg := <-msgChan:
    // 处理消息
case <-time.After(1 * time.Second):
    // 超时处理
case <-ctx.Done():
    // Context 取消
}
```

### 3. 自动重连

无需手动实现重连逻辑:

```go
client, _ := kcpq.Connect("localhost:4000")

// 启用自动重连（5秒间隔）
client.EnableAutoReconnect(5 * time.Second)

// 连接断开后会自动重连
// 所有订阅会在重连后自动恢复
```

**优势**:
- ✅ 无限重连
- ✅ 自动恢复所有订阅
- ✅ 减少代码量（从 160+ 行 → 110 行）
- ✅ 更可靠的连接管理

### 4. 增强统计信息

**21 个统计字段**:

```go
stats := client.GetStats()

// 连接状态
stats.Connected          // 是否已连接
stats.ConnectedAt        // 连接时间

// 消息统计
stats.MessagesSent       // 发送消息总数
stats.MessagesReceived   // 接收消息总数
stats.MessagesSentPerSec   // 发送速率 (msg/s)
stats.MessagesReceivedPerSec // 接收速率 (msg/s)

// 网络统计
stats.BytesSent          // 发送字节总数
stats.BytesReceived      // 接收字节总数
stats.AvgLatency         // 平均延迟
stats.LastLatency        // 最后一次延迟

// 订阅统计
stats.ActiveSubscriptions // 活跃订阅数
stats.TotalSubscriptions  // 总订阅数

// 错误统计
stats.ConnectionErrors    // 连接错误数
stats.PublishErrors       // 发布错误数
stats.SubscriptionErrors  // 订阅错误数
stats.ReconnectCount      // 重连次数 (v2.0 新增)
```

---

## 📊 性能基准

**测试环境**:
- 服务器: Intel Xeon, 16GB RAM
- 网络: 1 Gbps LAN
- 消息大小: 1 KB

**性能指标**:
- 吞吐量: **205+ fps** (每秒消息数)
- 并发连接: **5000+**
- 延迟: **<10ms** (p99)
- CPU 占用: <30% (单核)
- 内存占用: <100 MB

---

## 🔄 完整使用流程

### 场景: H.264 视频流传输和转发

#### 步骤 1: 启动 KCPQ 服务器

```bash
cd examples/server
go run main.go
```

#### 步骤 2: 启动 H.264 流转发

```bash
cd examples/h264-relay
go run main.go
```

#### 步骤 3: 启动 KCPQ 到 NATS 转发

```bash
cd examples/kcpq-to-nats
go run main.go
```

#### 步骤 4: 模拟 H.264 视频流输入

```bash
# 使用 ffmpeg 模拟 H.264 流
ffmpeg -re -i video.mp4 -f h264 udp://localhost:22345
```

---

## 🛠️ 常见问题

### Q: 如何处理连接断开?

**A**: v2.0 推荐使用自动重连:

```go
client.EnableAutoReconnect(5 * time.Second)
```

不需要手动实现重连逻辑，库会自动处理。

### Q: Channel 订阅 vs Callback 订阅如何选择?

**A**:
- **Channel 订阅**: 推荐，更符合 Go 惯用法，方便使用 select
- **Callback 订阅**: 向后兼容，简单场景可用

### Q: 如何优雅关闭客户端?

**A**: 使用 Context 取消:

```go
ctx, cancel := context.WithCancel(context.Background())
client, _ := kcpq.ConnectWithContext(ctx, "addr")

// 取消 Context
cancel()  // 会触发所有相关操作的清理

// 或直接关闭
client.Close()
```

### Q: 如何查看统计信息?

**A**:

```go
stats := client.GetStats()
fmt.Printf("Sent: %d, Received: %d, Latency: %v\n",
    stats.MessagesSent,
    stats.MessagesReceived,
    stats.AvgLatency,
)
```

---

## 📚 相关文档

- [KCPQ 主 README](../README.md)
- [API 文档](https://pkg.go.dev/github.com/kcpq/client)
- [重构指南](../docs/REFACTORING_GUIDE.md)
- [易用性分析](../docs/USABILITY_ANALYSIS.md)

---

## 🤝 贡献示例

欢迎提交新的示例程序！请确保:

1. ✅ 代码清晰，注释完整
2. ✅ 使用 KCPQ v2.0 API
3. ✅ 添加使用说明
4. ✅ 不包含敏感信息（IP、密码等）
5. ✅ 支持环境变量配置

---

## 📝 许可证

与 KCPQ 主项目相同

---

**KCPQ v2.0** - 高性能、低延迟的 Go 消息队列库 🚀
