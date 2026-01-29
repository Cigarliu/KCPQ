package server

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/xtaci/kcp-go"
)

// Server KCP-NATS 服务器
type Server struct {
	addr      string
	listener  *kcp.Listener
	router    *Router
	sessions  map[*Session]bool
	mu        sync.RWMutex
	done      chan struct{}
	closeOnce sync.Once

	block kcp.BlockCrypt

	sessionOutChanCap     int
	maxSessionQueuedBytes int64
	sessionWriteTimeout   time.Duration
}

// NewServer 创建新服务器
func NewServer(addr string, aes256Key []byte) (*Server, error) {
	block, err := newAES256BlockCrypt(aes256Key)
	if err != nil {
		return nil, err
	}
	return &Server{
		addr:                  addr,
		router:                NewRouter(),
		sessions:              make(map[*Session]bool),
		done:                  make(chan struct{}),
		block:                 block,
		sessionOutChanCap:     1024,
		maxSessionQueuedBytes: 32 * 1024 * 1024,
		sessionWriteTimeout:   5 * time.Second,
	}, nil
}

func (s *Server) SetSessionBackpressure(maxQueuedBytes int64, outChanCap int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxQueuedBytes > 0 {
		s.maxSessionQueuedBytes = maxQueuedBytes
	}
	if outChanCap > 0 {
		s.sessionOutChanCap = outChanCap
	}
}

func (s *Server) SetSessionWriteTimeout(timeout time.Duration) {
	s.mu.Lock()
	s.sessionWriteTimeout = timeout
	s.mu.Unlock()
}

// Start 启动服务器
func (s *Server) Start() error {
	var err error
	s.listener, err = kcp.ListenWithOptions(s.addr, s.block, 10, 3)
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
				if s.listener == nil {
					return
				}
				log.Printf("Accept error: %s", err)
				continue
			}

			session := NewSession(conn, s)

			s.mu.Lock()
			s.sessions[session] = true
			s.mu.Unlock()

			log.Printf("New connection from %s", conn.RemoteAddr())

			// 在 goroutine 中处理会话
			go func() {
				session.Start()
				s.removeSession(session)
			}()
		}
	}
}

// Close 关闭服务器
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)

		s.mu.Lock()
		defer s.mu.Unlock()

		for session := range s.sessions {
			session.Close()
			delete(s.sessions, session)
		}

		if s.listener != nil {
			err = s.listener.Close()
		}
	})

	return err
}

// GetStats 获取服务器统计信息
func (s *Server) GetStats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServerStats{
		Connections:        len(s.sessions),
		Subjects:           s.router.GetSubjectCount(),
		TotalSubscriptions: s.router.GetTotalSubscriptions(),
	}
}

// ServerStats 服务器统计信息
type ServerStats struct {
	Connections        int
	Subjects           int
	TotalSubscriptions int
}

// Addr 返回服务器地址
func (s *Server) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

func (s *Server) removeSession(session *Session) {
	s.mu.Lock()
	delete(s.sessions, session)
	s.mu.Unlock()
}
