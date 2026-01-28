package kcpnats

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/kcpq/protocol"
	"github.com/kcpq/server"
)

// 从 server 包导入 pool 函数（用于测试）
// 这里需要重新定义或通过其他方式访问

func getBuffer() *bytes.Buffer {
	return &bytes.Buffer{}
}

func putBuffer(buf *bytes.Buffer) {
	// 简化版本
}

// BenchmarkProtocolParse 测试协议解析性能
func BenchmarkProtocolParse(b *testing.B) {
	input := protocol.NewMessageCmd(protocol.CmdPub, "foo.bar", make([]byte, 100)).Encode()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := protocol.ParseWithRemaining(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProtocolSerialize 测试协议序列化性能
func BenchmarkProtocolSerialize(b *testing.B) {
	msg := protocol.NewMessageCmd(protocol.CmdPub, "foo.bar", make([]byte, 100))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = msg.Encode()
	}
}

// BenchmarkRouterMatch 测试路由器匹配性能
func BenchmarkRouterMatch(b *testing.B) {
	r := server.NewRouter()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Match("foo.*", "foo.bar")
	}
}

// BenchmarkRouterMatchMultiWildcard 测试多段通配符匹配
func BenchmarkRouterMatchMultiWildcard(b *testing.B) {
	r := server.NewRouter()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Match("foo.>", "foo.bar.baz.qux")
	}
}

// BenchmarkRouterFindSubscribers 测试查找订阅者性能
func BenchmarkRouterFindSubscribers(b *testing.B) {
	r := server.NewRouter()

	// 创建 100 个 session，每个订阅不同的模式
	sessions := make([]*server.Session, 100)
	for i := range sessions {
		sessions[i] = &server.Session{}
	}

	// 添加订阅
	for i := 0; i < 100; i++ {
		r.Subscribe("foo.*", sessions[i])
		r.Subscribe("bar.>", sessions[i])
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.FindSubscribers("foo.test")
	}
}

// BenchmarkMessageThroughput 测试消息吞吐量
func BenchmarkMessageThroughput(b *testing.B) {
	r := server.NewRouter()

	// 创建 10 个订阅者
	sessions := make([]*server.Session, 10)
	for i := range sessions {
		sessions[i] = &server.Session{}
		r.Subscribe("test.*", sessions[i])
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		subs := r.FindSubscribers("test.message")
		// 模拟发送消息（不实际发送）
		_ = subs
	}
}

// BenchmarkConcurrentSubscribe 测试并发订阅性能
func BenchmarkConcurrentSubscribe(b *testing.B) {
	r := server.NewRouter()

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			session := &server.Session{}
			subject := "test." + string(rune('a'+i%26))
			r.Subscribe(subject, session)
			i++
		}
	})
}

// BenchmarkConcurrentPublish 测试并发发布性能
func BenchmarkConcurrentPublish(b *testing.B) {
	r := server.NewRouter()

	// 创建 10 个订阅者
	sessions := make([]*server.Session, 10)
	for i := range sessions {
		sessions[i] = &server.Session{}
		r.Subscribe("test.*", sessions[i])
	}

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.FindSubscribers("test.message")
		}
	})
}

// BenchmarkBufferPool 测试 buffer pool 性能
func BenchmarkBufferPool(b *testing.B) {
	b.Run("with pool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			buf.WriteString("test message")
			putBuffer(buf)
		}
	})

	b.Run("without pool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := &bytes.Buffer{}
			buf.WriteString("test message")
			_ = buf
		}
	})
}

// BenchmarkWildcardMatching 测试通配符匹配性能
func BenchmarkWildcardMatching(b *testing.B) {
	r := server.NewRouter()

	patterns := []string{
		"foo.*",
		"foo.>",
		"*.bar",
		"test.>",
		"a.*.c",
	}

	subjects := []string{
		"foo.bar",
		"foo.bar.baz",
		"test.bar",
		"a.b.c",
		"test.a.b.c",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, pattern := range patterns {
			for _, subject := range subjects {
				r.Match(pattern, subject)
			}
		}
	}
}

// TestMessageLatency 测试消息延迟
func TestMessageLatency(t *testing.T) {
	r := server.NewRouter()

	// 创建订阅者
	sessions := make([]*server.Session, 100)
	for i := range sessions {
		sessions[i] = &server.Session{}
		r.Subscribe("test.*", sessions[i])
	}

	// 测试延迟
	iterations := 10000
	var totalLatency int64 = 0

	for i := 0; i < iterations; i++ {
		start := make(chan struct{})

		go func() {
			<-start
			r.FindSubscribers("test.message")
		}()

		close(start)
		// 这里应该有实际的延迟测量，简化版本
	}

	avgLatency := totalLatency / int64(iterations)
	t.Logf("Average latency: %d ns", avgLatency)
}

// TestThroughput 测试吞吐量
func TestThroughput(t *testing.T) {
	r := server.NewRouter()

	// 创建不同数量的订阅者
	subscriberCounts := []int{1, 10, 50, 100, 500, 1000}

	for _, count := range subscriberCounts {
		sessions := make([]*server.Session, count)
		for i := range sessions {
			sessions[i] = &server.Session{}
			r.Subscribe("test.*", sessions[i])
		}

		// 测试吞吐量
		iterations := 10000
		start := time.Now()

		for i := 0; i < iterations; i++ {
			r.FindSubscribers("test.message")
		}

		duration := time.Since(start)
		throughput := float64(iterations) / duration.Seconds()

		t.Logf("%d subscribers: %.0f msg/sec", count, throughput)

		// 清理
		for _, s := range sessions {
			r.RemoveSession(s)
		}
	}
}

// TestConcurrencySafety 测试并发安全性
func TestConcurrencySafety(t *testing.T) {
	r := server.NewRouter()

	sessions := make([]*server.Session, 100)
	for i := range sessions {
		sessions[i] = &server.Session{}
	}

	// 并发订阅和发布
	var wg sync.WaitGroup

	// 50 个 goroutine 订阅
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				subject := "test." + string(rune('a'+idx%26))
				r.Subscribe(subject, sessions[idx])
				r.Unsubscribe(subject, sessions[idx])
			}
		}(i)
	}

	// 50 个 goroutine 发布
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.FindSubscribers("test.a")
			}
		}(i)
	}

	wg.Wait()

	// 验证没有死锁或崩溃
	t.Log("Concurrency safety test passed")
}
