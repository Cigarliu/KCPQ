//go:build ignore

package main

import (
	"fmt"
	"log"

	"github.com/xtaci/kcp-go"
)

func main() {
	// 监听
	listener, err := kcp.ListenWithOptions(":4000", nil, 10, 3)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	log.Println("KCP Echo Server listening on :4000")

	for {
		// 接受连接
		conn, err := listener.AcceptKCP()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		log.Printf("New connection from %s", conn.RemoteAddr())

		// 设置参数
		conn.SetNoDelay(1, 20, 2, 1)
		conn.SetWindowSize(128, 128)
		conn.SetWriteDelay(false)
		conn.SetStreamMode(false)

		// 处理连接
		go handleConnection(conn)
	}
}

func handleConnection(conn *kcp.UDPSession) {
	defer conn.Close()

	buf := make([]byte, 4096)
	for {
		// 读取数据
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		// 打印收到的数据
		fmt.Printf("[SERVER] Received %d bytes: %s\n", n, string(buf[:n]))

		// 回显
		_, err = conn.Write(buf[:n])
		if err != nil {
			log.Printf("Write error: %v", err)
			return
		}
		fmt.Printf("[SERVER] Echoed %d bytes\n", n)
	}
}
