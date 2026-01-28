package server

import (
	"strings"
	"sync"
)

// OptimizedRouter 优化版路由器 - 借鉴 InMemoryMessageBus.cs 的设计
// 主要优化:
// 1. 分离精确匹配和通配符匹配，O(1) 精确查找
// 2. 使用预编译的模式匹配器
// 3. 减少内存分配
type OptimizedRouter struct {
	// 精确匹配的订阅 - O(1) 查找
	exactMatches map[string][]*Session

	// 通配符模式订阅 - 需要模式匹配
	wildcardPatterns map[string][]*Session

	mu sync.RWMutex
}

// NewOptimizedRouter 创建优化版路由器
func NewOptimizedRouter() *OptimizedRouter {
	return &OptimizedRouter{
		exactMatches:     make(map[string][]*Session),
		wildcardPatterns: make(map[string][]*Session),
	}
}

// CompiledPattern 预编译的模式 - 减少重复的字符串分割
type CompiledPattern struct {
	segments         []string
	hasWildcard      bool
	hasMultiWildcard bool
}

// compiledPatternCache 缓存编译后的模式
var compiledPatternCache = make(map[string]*CompiledPattern)
var cacheMu sync.RWMutex

// compilePattern 编译模式并缓存
func compilePattern(pattern string) *CompiledPattern {
	cacheMu.RLock()
	cached, ok := compiledPatternCache[pattern]
	cacheMu.RUnlock()

	if ok {
		return cached
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// 双重检查
	if cached, ok := compiledPatternCache[pattern]; ok {
		return cached
	}

	compiled := &CompiledPattern{
		segments: strings.Split(pattern, "."),
	}

	for _, seg := range compiled.segments {
		if seg == "*" {
			compiled.hasWildcard = true
		} else if seg == ">" {
			compiled.hasMultiWildcard = true
		}
	}

	compiledPatternCache[pattern] = compiled
	return compiled
}

// isWildcardPattern 检查是否包含通配符
func isWildcardPattern(pattern string) bool {
	return strings.Contains(pattern, "*") || strings.Contains(pattern, ">")
}

// Subscribe 订阅主题 - 优化版
func (r *OptimizedRouter) Subscribe(subject string, session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if isWildcardPattern(subject) {
		// 通配符订阅
		r.wildcardPatterns[subject] = append(r.wildcardPatterns[subject], session)
	} else {
		// 精确订阅
		r.exactMatches[subject] = append(r.exactMatches[subject], session)
	}

	// 预编译模式以提高后续匹配性能
	compilePattern(subject)
}

// Unsubscribe 取消订阅
func (r *OptimizedRouter) Unsubscribe(subject string, session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var targetMap map[string][]*Session
	if isWildcardPattern(subject) {
		targetMap = r.wildcardPatterns
	} else {
		targetMap = r.exactMatches
	}

	sessions := targetMap[subject]
	if sessions == nil {
		return
	}

	// 移除 session
	for i, s := range sessions {
		if s == session {
			targetMap[subject] = append(sessions[:i], sessions[i+1:]...)
			if len(targetMap[subject]) == 0 {
				delete(targetMap, subject)
			}
			return
		}
	}
}

// FindSubscribers 查找订阅者 - 优化版
// 先 O(1) 查找精确匹配，再匹配通配符模式
func (r *OptimizedRouter) FindSubscribers(subject string) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 使用 map 避免重复的 session
	seen := make(map[*Session]bool)
	result := make([]*Session, 0)

	// 1. 首先检查精确匹配 - O(1)
	if sessions, ok := r.exactMatches[subject]; ok {
		for _, session := range sessions {
			if !seen[session] {
				seen[session] = true
				result = append(result, session)
			}
		}
	}

	// 2. 然后检查通配符模式
	for pattern, sessions := range r.wildcardPatterns {
		compiled := compilePattern(pattern)
		if compiled.matches(subject) {
			for _, session := range sessions {
				if !seen[session] {
					seen[session] = true
					result = append(result, session)
				}
			}
		}
	}

	return result
}

// matches 检查编译后的模式是否匹配主题
// 优化: 避免重复的字符串分割
func (cp *CompiledPattern) matches(subject string) bool {
	if !cp.hasWildcard && !cp.hasMultiWildcard {
		// 没有通配符，精确匹配
		return subject == cp.segments[0]
	}

	// 需要分割主题
	subjectSegments := strings.Split(subject, ".")

	return cp.matchesSegments(subjectSegments)
}

// matchesSegments 检查模式是否匹配主题段
func (cp *CompiledPattern) matchesSegments(subjectSegments []string) bool {
	si := 0 // subject index

	for pi, patternSeg := range cp.segments {
		if patternSeg == ">" {
			// '>' 必须在最后一段
			if pi != len(cp.segments)-1 {
				return false
			}
			// 至少还要有一个主题段（foo.> 不匹配 foo）
			return si < len(subjectSegments)
		}

		if si >= len(subjectSegments) {
			// 主题段耗尽，但模式还没结束
			return false
		}

		if patternSeg == "*" {
			// '*' 匹配任意单段
			si++
			continue
		}

		// 精确匹配
		if patternSeg != subjectSegments[si] {
			return false
		}

		si++
	}

	// 模式用尽后，主题也必须恰好用尽
	return si == len(subjectSegments)
}

// Match 检查模式是否匹配主题 - 用于测试
func (r *OptimizedRouter) Match(pattern, subject string) bool {
	compiled := compilePattern(pattern)
	return compiled.matches(subject)
}

// RemoveSession 移除session的所有订阅
func (r *OptimizedRouter) RemoveSession(session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 从精确匹配中移除
	for subject, sessions := range r.exactMatches {
		for i, s := range sessions {
			if s == session {
				r.exactMatches[subject] = append(sessions[:i], sessions[i+1:]...)
				if len(r.exactMatches[subject]) == 0 {
					delete(r.exactMatches, subject)
				}
				break
			}
		}
	}

	// 从通配符模式中移除
	for pattern, sessions := range r.wildcardPatterns {
		for i, s := range sessions {
			if s == session {
				r.wildcardPatterns[pattern] = append(sessions[:i], sessions[i+1:]...)
				if len(r.wildcardPatterns[pattern]) == 0 {
					delete(r.wildcardPatterns, pattern)
				}
				break
			}
		}
	}
}

// GetSubjectCount 获取主题数量
func (r *OptimizedRouter) GetSubjectCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.exactMatches) + len(r.wildcardPatterns)
}

// GetTotalSubscriptions 获取总订阅数
func (r *OptimizedRouter) GetTotalSubscriptions() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, sessions := range r.exactMatches {
		count += len(sessions)
	}
	for _, sessions := range r.wildcardPatterns {
		count += len(sessions)
	}

	return count
}
