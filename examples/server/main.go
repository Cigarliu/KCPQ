package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kcpq/server"
)

func main() {
	// 启动 pprof
	go func() {
		log.Println("Pprof listening on :6060")
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	addr := os.Getenv("KCP_NATS_ADDR")
	if addr == "" {
		addr = ":4000"
	}
	srv := server.NewServer(addr)

	// 启动服务器
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %s", err)
	}

	// 打印统计信息
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stats := srv.GetStats()
			fmt.Printf("\n=== Server Stats ===\n")
			fmt.Printf("Connections: %d\n", stats.Connections)
			fmt.Printf("Subjects: %d\n", stats.Subjects)
			fmt.Printf("Total Subscriptions: %d\n", stats.TotalSubscriptions)
			fmt.Printf("===================\n")
		}
	}()

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down server...")
	if err := srv.Close(); err != nil {
		log.Printf("Error closing server: %s", err)
	}

	fmt.Println("Server stopped")
}
