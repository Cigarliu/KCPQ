package main

import (
	"encoding/hex"
	"flag"
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
	configPath := flag.String("config", "", "")
	flag.Parse()

	cfg := &ConfigFile{
		ListenAddr: ":4000",
		PprofAddr:  "localhost:6060",
	}

	cfgPath := *configPath
	if cfgPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			cfgPath = "config.yaml"
		}
	}
	if cfgPath != "" {
		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		cfg = loaded
	}

	if addr := os.Getenv("KCP_NATS_ADDR"); addr != "" {
		cfg.ListenAddr = addr
	}
	if pprofAddr := os.Getenv("KCPQ_PPROF_ADDR"); pprofAddr != "" {
		cfg.PprofAddr = pprofAddr
	}
	if keyHex := os.Getenv("KCPQ_AES256_KEY_HEX"); keyHex != "" {
		cfg.AES256KeyHex = keyHex
	}
	if cfg.AES256KeyHex == "" {
		log.Fatal("KCPQ_AES256_KEY_HEX (or config aes256_key_hex) is required (64 hex chars)")
	}
	key, err := hex.DecodeString(cfg.AES256KeyHex)
	if err != nil {
		log.Fatalf("invalid aes256_key_hex: %v", err)
	}
	if len(key) != 32 {
		log.Fatalf("aes256_key_hex must decode to 32 bytes, got %d", len(key))
	}

	go func() {
		log.Printf("Pprof listening on %s", cfg.PprofAddr)
		log.Println(http.ListenAndServe(cfg.PprofAddr, nil))
	}()
	srv, err := server.NewServer(cfg.ListenAddr, key)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

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
