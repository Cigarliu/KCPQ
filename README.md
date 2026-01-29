# KCPQ

KCPQ 是一个基于 KCP（可靠 UDP）的轻量级 Pub/Sub 消息分发组件，提供类似 NATS 的主题订阅模型（支持 `*` / `>` 通配符），面向低延迟、实时场景。

## 关键特性

- Pub/Sub：按 subject 发布与订阅，支持通配符 `*`（单段）与 `>`（多段）
- Context：关键操作支持超时与取消
- Channel 订阅：提供 `SubscribeChan*` 形式，符合 Go 习惯
- 自动重连：断线重连并恢复订阅
- 强制加密：KCP 层 AES-256 PSK 已强制启用（客户端/服务端必须共享同一把 32 字节密钥）

## 安装

```bash
go get github.com/kcpq
```

## 快速开始

### 1) 准备 AES-256 密钥（必需）

示例程序统一读取环境变量 `KCPQ_AES256_KEY_HEX`（64 个 hex 字符，解码后必须为 32 bytes）：

```bash
export KCPQ_AES256_KEY_HEX=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

### 2) 启动服务端

```bash
cd examples/server
go run .
```

默认监听 `:4000/udp`（可通过 `KCP_NATS_ADDR` 覆盖）。

### 3) 运行订阅者与发布者

```bash
cd examples/subscriber
go run .
```

```bash
cd examples/publisher
go run .
```

## API 速览

服务端：

- `server.NewServer(addr, aes256Key) (*server.Server, error)`

客户端：

- `client.ConnectWithContext(ctx, addr, aes256Key) (*client.Client, error)`
- `client.Connect(addr, aes256Key) (*client.Client, error)`

## 文档与示例

- 示例入口与说明：[examples/README.md](examples/README.md)
- 文档目录：[docs/README.md](docs/README.md)
  - 生产部署：[docs/生产环境部署指南.md](docs/生产环境部署指南.md)
  - 项目状态：[docs/项目状态总结.md](docs/项目状态总结.md)

