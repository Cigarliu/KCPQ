# KCPQ

KCPQ 是一个基于 KCP（可靠 UDP）的高性能 Pub/Sub 消息队列，提供类似 NATS 的主题订阅模型，专为低延迟、实时场景设计。

## 核心特性

- **高性能 Pub/Sub**：基于 KCP 可靠 UDP 协议，延迟远低于传统 TCP 方案
- **灵活的主题订阅**：支持 `*`（单段通配符）和 `>`（多段通配符）
- **安全加密**：强制启用 AES-256 PSK 加密，确保数据传输安全
- **自动重连**：网络断开时自动重连并恢复订阅
- **Go 原生支持**：提供 Channel 订阅接口，完美融入 Go 并发模型
- **Context 控制**：关键操作支持超时和取消

## 典型应用场景

- **实时视频流**：H.264/H.265 视频帧分发（已验证支持 240+ fps）
- **实时数据推送**：金融行情、IoT 传感器数据、游戏状态同步
- **消息桥接**：作为边缘消息队列，桥接本地数据到中心 NATS/Kafka
- **低延迟 RPC**：需要超低延迟的远程调用场景

## 快速入门

### 前置要求

- Go 1.19+
- 了解 Go 基础语法和并发概念

### 5 分钟上手体验

#### 1. 克隆仓库

```bash
git clone https://github.com/Cigarliu/KCPQ.git
cd KCPQ
```

#### 2. 生成 AES-256 密钥

**重要**：所有组件必须使用相同的 32 字节密钥（64 个 hex 字符）

```bash
# 生成新密钥（推荐）
openssl rand -hex 32

# 或使用测试密钥（仅用于开发环境）
export KCPQ_AES256_KEY_HEX=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

#### 3. 启动服务端

```bash
cd examples/server
go run .
```

服务端将监听 `:4000/udp`，输出类似：

```
[INFO] Starting KCPQ server on :4000
[INFO] AES-256 encryption enabled
```

#### 4. 运行订阅者

打开新终端：

```bash
cd KCPQ/examples/subscriber
go run .
```

订阅者将订阅 `test.subject`，等待接收消息。

#### 5. 运行发布者

再打开一个新终端：

```bash
cd KCPQ/examples/publisher
go run .
```

发布者每秒向 `test.subject` 发送一条消息，你应该能在订阅者终端看到接收到的消息。

### 基本编程示例

#### 服务端代码

```go
package main

import (
    "log"
    "github.com/Cigarliu/KCPQ/server"
)

func main() {
    // 创建服务端（必须提供 32 字节 AES-256 密钥）
    aesKey := []byte("your-32-byte-aes-key-here!!!!!!")
    srv, err := server.NewServer(":4000", aesKey)
    if err != nil {
        log.Fatal(err)
    }

    // 启动服务（阻塞运行）
    log.Println("KCPQ server started on :4000")
    srv.Run()
}
```

#### 客户端代码 - 发布消息

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/Cigarliu/KCPQ/client"
)

func main() {
    // 连接服务器（必须使用与服务端相同的密钥）
    aesKey := []byte("your-32-byte-aes-key-here!!!!!!")
    cli, err := client.Connect("localhost:4000", aesKey)
    if err != nil {
        log.Fatal(err)
    }
    defer cli.Close()

    // 发布消息
    ctx := context.Background()
    for i := 0; i < 10; i++ {
        msg := []byte(fmt.Sprintf("Hello KCPQ #%d", i))
        if err := cli.Publish(ctx, "test.subject", msg); err != nil {
            log.Printf("Publish failed: %v", err)
        }
        time.Sleep(1 * time.Second)
    }
}
```

#### 客户端代码 - 订阅消息

```go
package main

import (
    "context"
    "log"
    "github.com/Cigarliu/KCPQ/client"
)

func main() {
    // 连接服务器
    aesKey := []byte("your-32-byte-aes-key-here!!!!!!")
    cli, err := client.Connect("localhost:4000", aesKey)
    if err != nil {
        log.Fatal(err)
    }
    defer cli.Close()

    // 订阅主题（使用 Channel 接收消息）
    ctx := context.Background()
    sub, err := cli.SubscribeChan(ctx, "test.subject")
    if err != nil {
        log.Fatal(err)
    }

    // 接收消息
    for msg := range sub.Ch {
        log.Printf("Received: %s", msg.Data)
    }
}
```

### 进阶使用

#### 通配符订阅

```go
// 订阅单个通配符：匹配 "foo.bar", "foo.baz"，但不匹配 "foo.bar.baz"
sub1, _ := cli.SubscribeChan(ctx, "foo.*")

// 订阅多个通配符：匹配 "foo.bar", "foo.bar.baz", "foo.bar.baz.qux"
sub2, _ := cli.SubscribeChan(ctx, "foo.>")
```

#### 使用超时控制

```go
// 设置 5 秒超时
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// 在超时后操作会自动取消
err := cli.Publish(ctx, "test.subject", data)
if errors.Is(err, context.DeadlineExceeded) {
    log.Println("Publish timeout")
}
```

#### 启用自动重连

```go
// 客户端默认启用自动重连
// 断线后会自动重连并恢复所有订阅
// 可通过日志观察重连过程
```

### 实战示例：H.264 视频流转发

KCPQ 已在生产环境中验证支持 240+ fps 的 H.264 视频流转发：

```bash
# 服务端（Linux）
cd examples/server
GOOS=linux GOARCH=amd64 go build -o kcpq-server .
scp kcpq-server user@server:/opt/kcpq/
ssh user@server "/opt/kcpq/kcpq-server"

# 发布端（接收 UDP 视频流并转发到 KCPQ）
cd examples/h264-relay
go run . -config config.yaml

# 桥接端（从 KCPQ 订阅并转发到 NATS）
cd examples/kcpq-to-nats
go run . -config config.yaml
```

完整生产部署指南请参考：[docs/生产环境部署指南.md](docs/生产环境部署指南.md)

## 常见问题排查

### 1. 连接失败：握手超时

**症状**：客户端显示 "Connected" 但服务端无连接记录

**原因**：AES-256 密钥不匹配

**解决**：
```bash
# 确保所有组件使用完全相同的密钥
export KCPQ_AES256_KEY_HEX=$(openssl rand -hex 32)
echo $KCPQ_AES256_KEY_HEX  # 记录此密钥

# 在所有服务端和客户端使用相同的环境变量或配置文件
```

### 2. 订阅超时

**症状**：`Subscription ACK timeout after 5s`

**原因**：网络不可达或防火墙阻止 UDP 端口

**解决**：
```bash
# 检查服务端端口
netstat -tuln | grep 4000

# 检查防火墙（Linux）
sudo ufw allow 4000/udp
sudo iptables -A INPUT -p udp --dport 4000 -j ACCEPT

# 测试连通性
telnet server_ip 4000  # 即使失败也要确认端口可达
```

### 3. 编译错误

**症状**：`undefined: ConfigFile`

**解决**：
```bash
# 手动编译并验证无错误输出
go build -v . 2>&1 | grep -i error
# 或
go build -ldflags="-s -w" -o myapp .
```

### 4. 性能优化

**高吞吐场景**（视频流、高频数据）：
- 使用 Channel 订阅模式（`SubscribeChan`）而非回调模式
- 调整 KCP 参数（需修改代码）
- 考虑多路复用（一个连接处理多个主题）

**低延迟场景**：
- 确保网络质量良好（KCP 在丢包网络中优势更明显）
- 减小消息大小
- 使用 Context 超时避免阻塞

## API 速览

### 服务端 API

```go
// 创建服务端
server.NewServer(addr string, aes256Key []byte) (*server.Server, error)

// 启动服务（阻塞）
srv.Run() error

// 优雅关闭
srv.Close()
```

### 客户端 API

```go
// 连接服务器
client.Connect(addr string, aes256Key []byte) (*client.Client, error)
client.ConnectWithContext(ctx context.Context, addr string, aes256Key []byte) (*client.Client, error)

// 发布消息
cli.Publish(ctx context.Context, subject string, data []byte) error

// 订阅（回调模式）
cli.Subscribe(ctx context.Context, subject string, handler func(msg *Message)) (*Subscription, error)

// 订阅（Channel 模式，推荐）
cli.SubscribeChan(ctx context.Context, subject string) (*Subscription, error)

// 取消订阅
sub.Unsubscribe() error

// 关闭连接
cli.Close() error
```

### 数据结构

```go
// 消息结构
type Message struct {
    Subject string    // 主题
    Data    []byte    // 数据
    Time    time.Time // 时间戳
}

// 订阅对象
type Subscription struct {
    Ch chan *Message  // 消息通道（Channel 模式）
    // ...
}
```

## 文档与示例

### 示例程序

- **[examples/README.md](examples/README.md)** - 所有示例程序的完整说明
  - `server/` - 基础服务端
  - `publisher/` - 消息发布示例
  - `subscriber/` - 消息订阅示例
  - `h264-relay/` - H.264 视频流转发（生产级）
  - `kcpq-to-nats/` - KCPQ 到 NATS 桥接
  - `ping_pong/` - 性能测试工具
  - `comprehensive_test/` - 综合测试套件

### 文档

- **[docs/生产环境部署指南.md](docs/生产环境部署指南.md)** - 生产环境部署完整指南
  - 编译、配置、防火墙、systemd 服务
  - 故障排查流程和实战经验
- **[docs/项目状态总结.md](docs/项目状态总结.md)** - 项目当前状态和测试覆盖

## 技术架构

### 协议栈

```
应用层：Pub/Sub 消息模型
      ↓
加密层：AES-256 PSK（强制）
      ↓
传输层：KCP（可靠 UDP）
      ↓
网络层：UDP
```

### 核心优势

- **低延迟**：基于 UDP，避免了 TCP 的队头阻塞
- **可靠传输**：KCP 协议保证数据可靠性和顺序
- **安全加密**：强制 AES-256 加密，防止数据泄露
- **灵活订阅**：支持通配符，适应复杂场景
- **易于集成**：Go 原生 API，符合并发模型

## 许可证

本项目采用 MIT 许可证。

## 贡献

欢迎提交 Issue 和 Pull Request！

## 相关资源

- [KCP 协议](https://github.com/skywind3000/kcp) - 底层可靠传输协议
- [NATS](https://nats.io/) - 灵感来源的高性能消息系统

