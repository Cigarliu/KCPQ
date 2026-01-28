package server

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/xtaci/kcp-go"
)

// Server KCP-NATS 服务器
type Server struct {
	addr   string
	listener *kcp.Listener
	router *Router
	sessions map[*Session]bool
	mu      sync.RWMutex
	done    chan struct{}
}

// NewServer 创建新服务器
func NewServer(addr string) *Server {
	return &Server{
		addr:     addr,
		router:   NewRouter(),
		sessions: make(map[*Session]bool),
		done:     make(chan struct{}),
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	var err error
	s.listener, err = kcp.ListenWithOptions(s.addr, nil, 10, 3)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	log.Printf("KCP-NATS server listening on %s", s.addr)

	// 接受连接
	go s.acceptLoop()

	return nil
}

// acceptLoop 接受连接循环
func (s *Server) acceptLoop() {
	for {
		select {
		case <-s.done:
			return
		default:
			conn, err := s.listener.AcceptKCP()
			if err != nil {
				log.Printf("Accept error: %s", err)
				continue
			}

			session := NewSession(conn, s)

			s.mu.Lock()
			s.sessions[session] = true
			s.mu.Unlock()

			log.Printf("New connection from %s", conn.RemoteAddr())

			// 在 goroutine 中处理会话
			go session.Start()
		}
	}
}

// Close 关闭服务器
func (s *Server) Close() error {
	close(s.done)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有会话
	for session := range s.sessions {
		session.Close()
		delete(s.sessions, session)
	}

	if s.listener != nil {
		return s.listener.Close()
	}

	return nil
}

// GetStats 获取服务器统计信息
func (s *Server) GetStats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServerStats{
		Connections:         len(s.sessions),
		Subjects:            s.router.GetSubjectCount(),
		TotalSubscriptions:  s.router.GetTotalSubscriptions(),
	}
}

// ServerStats 服务器统计信息
type ServerStats struct {
	Connections         int
	Subjects            int
	TotalSubscriptions  int
}

// Addr 返回服务器地址
func (s *Server) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}
