package server

import (
	"sync"
	"testing"
)

func TestRouterMatch(t *testing.T) {
	router := NewRouter()

	tests := []struct {
		name     string
		pattern  string
		subject  string
		wantMatch bool
	}{
		// 精确匹配
		{
			name:     "exact match",
			pattern:  "foo.bar",
			subject:  "foo.bar",
			wantMatch: true,
		},
		{
			name:     "exact match - different",
			pattern:  "foo.bar",
			subject:  "foo.baz",
			wantMatch: false,
		},

		// * 通配符（单段）
		{
			name:     "wildcard single segment",
			pattern:  "foo.*",
			subject:  "foo.bar",
			wantMatch: true,
		},
		{
			name:     "wildcard single segment - different",
			pattern:  "foo.*",
			subject:  "foo.bar.baz",
			wantMatch: false,
		},
		{
			name:     "wildcard in middle",
			pattern:  "*.bar",
			subject:  "foo.bar",
			wantMatch: true,
		},
		{
			name:     "wildcard - no match",
			pattern:  "foo.*",
			subject:  "bar.foo",
			wantMatch: false,
		},

		// > 通配符（多段）
		{
			name:     "multi-wildcard simple",
			pattern:  "foo.>",
			subject:  "foo.bar",
			wantMatch: true,
		},
		{
			name:     "multi-wildcard multiple segments",
			pattern:  "foo.>",
			subject:  "foo.bar.baz",
			wantMatch: true,
		},
		{
			name:     "multi-wildcard - no match",
			pattern:  "foo.>",
			subject:  "bar.foo",
			wantMatch: false,
		},
		{
			name:     "multi-wildcard - exact prefix",
			pattern:  "foo.>",
			subject:  "foo",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.Match(tt.pattern, tt.subject)
			if got != tt.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestRouterSubscribeUnsubscribe(t *testing.T) {
	router := NewRouter()

	// 创建测试 session
	session1 := &Session{}
	session2 := &Session{}

	// 测试订阅
	router.Subscribe("foo.bar", session1)
	router.Subscribe("foo.*", session2)
	router.Subscribe("test.>", session1)

	if router.GetSubjectCount() != 3 {
		t.Errorf("After Subscribe: got %d subjects, want 3", router.GetSubjectCount())
	}

	if router.GetTotalSubscriptions() != 3 {
		t.Errorf("After Subscribe: got %d total subscriptions, want 3", router.GetTotalSubscriptions())
	}

	// 测试取消订阅
	router.Unsubscribe("foo.bar", session1)

	if router.GetSubjectCount() != 2 {
		t.Errorf("After Unsubscribe: got %d subjects, want 2", router.GetSubjectCount())
	}

	if router.GetTotalSubscriptions() != 2 {
		t.Errorf("After Unsubscribe: got %d total subscriptions, want 2", router.GetTotalSubscriptions())
	}
}

func TestRouterFindSubscribers(t *testing.T) {
	router := NewRouter()

	// 创建测试 session
	session1 := &Session{}
	session2 := &Session{}
	session3 := &Session{}

	// 设置订阅
	router.Subscribe("foo.bar", session1)
	router.Subscribe("foo.*", session2)
	router.Subscribe("test.>", session3)
	router.Subscribe("foo.>", session1)

	// 测试查找订阅者
	t.Run("find exact match", func(t *testing.T) {
		subs := router.FindSubscribers("foo.bar")

		// foo.bar 应该匹配: foo.bar (session1), foo.* (session2), foo.> (session1)
		// session1 会出现两次（在 foo.bar 和 foo.>），但应该去重
		if len(subs) != 2 {
			t.Errorf("FindSubscribers() returned %d sessions, want 2", len(subs))
		}
	})

	t.Run("find wildcard match", func(t *testing.T) {
		subs := router.FindSubscribers("foo.baz")

		// foo.baz 应该匹配: foo.* (session2), foo.> (session1)
		if len(subs) != 2 {
			t.Errorf("FindSubscribers() returned %d sessions, want 2", len(subs))
		}
	})

	t.Run("find multi-wildcard match", func(t *testing.T) {
		subs := router.FindSubscribers("test.a.b.c")

		// test.a.b.c 应该匹配: test.> (session3)
		if len(subs) != 1 {
			t.Errorf("FindSubscribers() returned %d sessions, want 1", len(subs))
		}
	})

	t.Run("find no match", func(t *testing.T) {
		subs := router.FindSubscribers("unknown.subject")
		if len(subs) != 0 {
			t.Errorf("FindSubscribers() returned %d sessions, want 0", len(subs))
		}
	})
}

func TestRouterConcurrency(t *testing.T) {
	router := NewRouter()

	// 创建多个 sessions
	sessions := make([]*Session, 100)
	for i := range sessions {
		sessions[i] = &Session{}
	}

	// 并发订阅
	var wg sync.WaitGroup
	for i, session := range sessions {
		wg.Add(1)
		go func(idx int, s *Session) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				subject := string(rune('a'+idx%26)) + ".test." + string(rune('0'+j))
				router.Subscribe(subject, s)
			}
		}(i, session)
	}

	wg.Wait()

	// 验证订阅数
	expectedSubs := 100 * 10
	if router.GetTotalSubscriptions() != expectedSubs {
		t.Errorf("After concurrent Subscribe: got %d total subscriptions, want %d",
			router.GetTotalSubscriptions(), expectedSubs)
	}

	// 并发查找
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			subject := string(rune('a'+idx%26)) + ".test.5"
			router.FindSubscribers(subject)
		}(i)
	}

	wg.Wait()

	// 验证订阅数没有变化
	if router.GetTotalSubscriptions() != expectedSubs {
		t.Errorf("After concurrent FindSubscribers: got %d total subscriptions, want %d",
			router.GetTotalSubscriptions(), expectedSubs)
	}
}

func BenchmarkRouterMatch(b *testing.B) {
	router := NewRouter()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		router.Match("foo.*", "foo.bar")
	}
}

func BenchmarkRouterFindSubscribers(b *testing.B) {
	router := NewRouter()

	// 设置 100 个订阅
	for i := 0; i < 100; i++ {
		session := &Session{}
		router.Subscribe("test.*", session)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		router.FindSubscribers("test.foo")
	}
}
