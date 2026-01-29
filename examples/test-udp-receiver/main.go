package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	// 监听 UDP 端口 22345
	addr, err := net.ResolveUDPAddr("udp", ":22345")
	if err != nil {
		fmt.Printf("Failed to resolve address: %v\n", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Failed to listen on UDP 22345: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("========================================")
	fmt.Println("UDP Port 22345 Listener")
	fmt.Println("========================================")
	fmt.Println("Listening for UDP packets on port 22345...")
	fmt.Println("Will display statistics every 5 seconds")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("")

	buffer := make([]byte, 65535)
	totalPackets := 0
	totalBytes := 0
	startTime := time.Now()

	// 每 5 秒打印统计
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			duration := time.Since(startTime).Seconds()
			packetsPerSec := float64(totalPackets) / duration
			bytesPerSec := float64(totalBytes) / duration
			mbps := (bytesPerSec * 8) / (1000 * 1000)

			fmt.Printf("[%s] Total: %d packets, %d bytes | Rate: %.2f pps, %.2f KB/s, %.3f Mbps\n",
				time.Now().Format("15:04:05"),
				totalPackets,
				totalBytes,
				packetsPerSec,
				bytesPerSec/1024,
				mbps)
		}
	}()

	// 显示每次收到的数据包
	packetCount := 0
	for {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		n, src, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Printf("[%s] Timeout - no data for 10 seconds!\n", time.Now().Format("15:04:05"))
				continue
			}
			fmt.Printf("Read error: %v\n", err)
			return
		}

		packetCount++
		totalPackets++
		totalBytes += n

		// 每 100 个包打印一次
		if packetCount <= 10 || packetCount%100 == 0 {
			fmt.Printf("[%s] Packet #%d from %s:%d -> %d bytes\n",
				time.Now().Format("15:04:05"),
				totalPackets,
				src.IP, src.Port,
				n)
		}
	}
}
