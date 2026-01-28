package client

import (
	"testing"
	"time"

	"github.com/kcpq/protocol"
)

// TestMatchSubject 测试主题匹配逻辑
func TestMatchSubject(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		subject  string
		expected bool
	}{
		// 精确匹配
		{"Exact match", "foo.bar", "foo.bar", true},
		{"Exact match different", "foo.bar", "foo.baz", false},

		// 通配符 * (单段匹配)
		{"Wildcard single segment", "foo.*", "foo.bar", true},
		{"Wildcard single segment prefix", "*.bar", "test.bar", true},
		{"Wildcard single segment multiple", "foo.*.baz", "foo.bar.baz", true},
		{"Wildcard no match", "foo.*", "bar.foo", false},

		// 通配符 > (多段匹配)
		{"Wildcard multi segment", "foo.>", "foo.bar", true},
		{"Wildcard multi segment deep", "foo.>", "foo.bar.baz.qux", true},
		{"Wildcard multi segment exact", "foo.>", "foo", true},
		{"Wildcard multi segment no match", "bar.>", "foo.bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchSubject(tt.pattern, tt.subject)
			if result != tt.expected {
				t.Errorf("matchSubject(%q, %q) = %v, want %v",
					tt.pattern, tt.subject, result, tt.expected)
			}
		})
	}
}

// TestSubscriptionACK 测试订阅ACK结构
func TestSubscriptionACK(t *testing.T) {
	ack := &subscriptionACK{
		subject: "test.subject",
		acked:   make(chan struct{}),
		timeout: time.Now().Add(5 * time.Second),
	}

	if ack.subject != "test.subject" {
		t.Errorf("Expected subject 'test.subject', got %s", ack.subject)
	}

	// 测试通道未关闭
	select {
	case <-ack.acked:
		t.Error("acked channel should not be closed initially")
	default:
		// 正常
	}

	// 关闭通道
	close(ack.acked)

	// 测试通道已关闭
	select {
	case <-ack.acked:
		// 正常 - 通道已关闭
	default:
		t.Error("acked channel should be closed after close()")
	}
}

// TestClientStats 测试客户端统计
func TestClientStats(t *testing.T) {
	stats := ClientStats{
		ActiveSubscriptions: 5,
	}

	if stats.ActiveSubscriptions != 5 {
		t.Errorf("Expected 5 active subscriptions, got %d", stats.ActiveSubscriptions)
	}
}

// TestConnectionState 测试连接状态
func TestConnectionState(t *testing.T) {
	if StateConnected != 0 {
		t.Errorf("Expected StateConnected to be 0, got %d", StateConnected)
	}

	if StateDisconnected != 1 {
		t.Errorf("Expected StateDisconnected to be 1, got %d", StateDisconnected)
	}
}

// TestProtocolCommands 测试协议命令常量
func TestProtocolCommands(t *testing.T) {
	// 确保协议命令常量存在
	_ = protocol.CmdMsg
	_ = protocol.CmdSub
	_ = protocol.CmdPub
	_ = protocol.CmdOk
	_ = protocol.CmdErr
	_ = protocol.CmdPing
	_ = protocol.CmdPong
}
