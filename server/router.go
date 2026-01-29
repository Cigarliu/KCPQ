package server

import (
	"strings"
	"sync"
)

// Router 处理 subject 路由和通配符匹配
type Router struct {
	subscriptions map[string][]*Session // subject -> sessions
	mu            sync.RWMutex
}

// NewRouter 创建新的路由器
func NewRouter() *Router {
	return &Router{
		subscriptions: make(map[string][]*Session),
	}
}

// Subscribe 订阅一个 subject
func (r *Router) Subscribe(subject string, session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions := r.subscriptions[subject]
	for _, s := range sessions {
		if s == session {
			return
		}
	}
	r.subscriptions[subject] = append(sessions, session)
}

// Unsubscribe 取消订阅
func (r *Router) Unsubscribe(subject string, session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions, ok := r.subscriptions[subject]
	if !ok {
		return
	}

	filtered := sessions[:0]
	for _, s := range sessions {
		if s != session {
			filtered = append(filtered, s)
		}
	}
	r.subscriptions[subject] = filtered

	// 如果没有订阅者了，删除 key
	if len(r.subscriptions[subject]) == 0 {
		delete(r.subscriptions, subject)
	}
}

// RemoveSession 移除 session 的所有订阅
func (r *Router) RemoveSession(session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for subject, sessions := range r.subscriptions {
		filtered := sessions[:0]
		for _, s := range sessions {
			if s != session {
				filtered = append(filtered, s)
			}
		}
		r.subscriptions[subject] = filtered
		if len(r.subscriptions[subject]) == 0 {
			delete(r.subscriptions, subject)
		}
	}
}

// Match 检查 subject 是否匹配 pattern（支持通配符）
func (r *Router) Match(pattern, subject string) bool {
	if pattern == subject {
		return true
	}

	// 处理 > 通配符（多段匹配）
	if strings.HasSuffix(pattern, ".>") {
		prefix := strings.TrimSuffix(pattern, ".>")
		if strings.HasPrefix(subject, prefix+".") || subject == prefix {
			return true
		}
		return false
	}

	// 处理 * 通配符（单段匹配）
	patternParts := strings.Split(pattern, ".")
	subjectParts := strings.Split(subject, ".")

	if len(patternParts) != len(subjectParts) {
		return false
	}

	// 检查是否有通配符
	hasWildcard := false
	for _, part := range patternParts {
		if part == "*" {
			hasWildcard = true
			break
		}
	}

	if !hasWildcard {
		return false
	}

	// 逐段匹配
	for i := 0; i < len(patternParts); i++ {
		if patternParts[i] == "*" {
			continue
		}
		if patternParts[i] != subjectParts[i] {
			return false
		}
	}

	return true
}

// FindSubscribers 找到所有匹配的订阅者
func (r *Router) FindSubscribers(subject string) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 使用 map 去重
	uniqueSessions := make(map[*Session]bool)

	for pattern, sessions := range r.subscriptions {
		if r.Match(pattern, subject) {
			for _, session := range sessions {
				uniqueSessions[session] = true
			}
		}
	}

	result := make([]*Session, 0, len(uniqueSessions))
	for session := range uniqueSessions {
		result = append(result, session)
	}

	return result
}

// GetSubjectCount 获取订阅的 subject 数量
func (r *Router) GetSubjectCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscriptions)
}

// GetTotalSubscriptions 获取总订阅数
func (r *Router) GetTotalSubscriptions() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, sessions := range r.subscriptions {
		count += len(sessions)
	}
	return count
}
