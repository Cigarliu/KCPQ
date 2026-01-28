//go:build ignore

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/xtaci/kcp-go"
)

func main() {
	// 连接服务器
	conn, err := kcp.DialWithOptions("localhost:4000", nil, 10, 3)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// 设置参数
	conn.SetNoDelay(1, 20, 2, 1)
	conn.SetWindowSize(128, 128)
	conn.SetWriteDelay(false)
	conn.SetStreamMode(false)

	log.Println("Connected to server")

	// 发送测试消息
	messages := []string{
		"Hello KCP!",
		"Test message 2",
		"Test message 3",
	}

	for _, msg := range messages {
		// 发送
		data := []byte(msg)
		fmt.Printf("[CLIENT] Sending: %s (%d bytes)\n", msg, len(data))

		n, err := conn.Write(data)
		if err != nil {
			log.Printf("Write error: %v", err)
			return
		}
		fmt.Printf("[CLIENT] Wrote %d bytes\n", n)

		// 接收回显
		buf := make([]byte, 4096)
		n, err = conn.Read(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		fmt.Printf("[CLIENT] Received echo: %s (%d bytes)\n", string(buf[:n]), n)
		fmt.Println("---")

		// 等待一下
		time.Sleep(500 * time.Millisecond)
	}

	log.Println("Test completed")
}
