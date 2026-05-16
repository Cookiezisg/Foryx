// Package ask is the in-memory rendezvous between the AskUserQuestion tool and the answer-delivery HTTP endpoint.
//
// Package ask 是 AskUserQuestion 工具与答案投递 HTTP 端点之间的内存会合点。
package ask

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrNoPendingQuestion: Resolve targeted a tool_call_id with no waiting Wait.
	//
	// ErrNoPendingQuestion：Resolve 找不到对应 tool_call_id 的 pending Wait。
	ErrNoPendingQuestion = errors.New("ask: no pending question for that tool_call_id")

	// ErrTimeout: Wait deadline elapsed before an answer arrived.
	//
	// ErrTimeout：Wait 超时仍无答案。
	ErrTimeout = errors.New("ask: user did not respond within the timeout")
)

// Service owns the in-memory rendezvous registry; methods are safe for concurrent use.
//
// Service 持有内存会合注册表，方法并发安全。
type Service struct {
	mu      sync.Mutex
	pending map[string]chan string
}

// NewService returns an empty Service ready to register questions.
//
// NewService 返回空 Service。
func NewService() *Service {
	return &Service{pending: make(map[string]chan string)}
}

// Wait registers a pending question and blocks until answered, cancelled, or timed out; registry entry is always cleaned up.
//
// Wait 注册 pending 问题并阻塞，直至答案到达 / ctx 取消 / 超时；返回前必清理注册表。
func (s *Service) Wait(ctx context.Context, toolCallID string, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)

	s.mu.Lock()
	if _, exists := s.pending[toolCallID]; exists {
		s.mu.Unlock()
		return "", errors.New("ask: tool_call_id already pending — caller bug")
	}
	s.pending[toolCallID] = ch
	s.mu.Unlock()

	defer s.cleanup(toolCallID)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ans := <-ch:
		return ans, nil
	case <-timer.C:
		return "", ErrTimeout
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Resolve delivers the answer to a waiting Wait and atomically removes the entry; second Resolve gets ErrNoPendingQuestion.
//
// Resolve 投递答案并原子删条目，第二次 Resolve 必拿 ErrNoPendingQuestion。
func (s *Service) Resolve(toolCallID, answer string) error {
	s.mu.Lock()
	ch, ok := s.pending[toolCallID]
	if ok {
		delete(s.pending, toolCallID)
	}
	s.mu.Unlock()
	if !ok {
		return ErrNoPendingQuestion
	}
	ch <- answer
	return nil
}

func (s *Service) cleanup(toolCallID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, toolCallID)
}

func (s *Service) pendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}
