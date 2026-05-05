// ask_test.go — unit tests for the ask Service rendezvous: Wait blocks,
// Resolve unblocks, timeouts and ctx-cancellation behave, double-resolve
// errors cleanly, and the registry is always cleaned up.
//
// ask_test.go — ask Service 会合单测：Wait 阻塞、Resolve 解锁、超时与
// ctx 取消语义、二次 Resolve 报错、注册表恒清理。
package ask

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestService_WaitResolves_RoundTrip(t *testing.T) {
	svc := NewService()

	answerCh := make(chan string, 1)
	go func() {
		ans, err := svc.Wait(context.Background(), "call_001", 2*time.Second)
		if err != nil {
			t.Errorf("Wait err: %v", err)
		}
		answerCh <- ans
	}()
	// Tiny pause so Wait has registered the channel before Resolve runs.
	// 让 Wait 先把 channel 注册好。
	time.Sleep(10 * time.Millisecond)
	if err := svc.Resolve("call_001", "user said yes"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	select {
	case ans := <-answerCh:
		if ans != "user said yes" {
			t.Errorf("answer = %q", ans)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after Resolve")
	}
	if got := svc.pendingCount(); got != 0 {
		t.Errorf("registry not cleaned up: %d pending", got)
	}
}

func TestService_Wait_Timeout(t *testing.T) {
	svc := NewService()
	start := time.Now()
	_, err := svc.Wait(context.Background(), "call_timeout", 50*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("want ErrTimeout, got %v", err)
	}
	if elapsed < 40*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("unexpected elapsed: %v", elapsed)
	}
	if got := svc.pendingCount(); got != 0 {
		t.Errorf("registry not cleaned up after timeout: %d pending", got)
	}
}

func TestService_Wait_CtxCancelled(t *testing.T) {
	svc := NewService()
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() {
		_, err := svc.Wait(ctx, "call_cancel", 5*time.Second)
		doneCh <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-doneCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("want ctx.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after ctx cancel")
	}
	if got := svc.pendingCount(); got != 0 {
		t.Errorf("registry not cleaned up after cancel: %d pending", got)
	}
}

func TestService_Resolve_UnknownIDIsErrNoPendingQuestion(t *testing.T) {
	svc := NewService()
	err := svc.Resolve("call_doesnotexist", "x")
	if !errors.Is(err, ErrNoPendingQuestion) {
		t.Errorf("want ErrNoPendingQuestion, got %v", err)
	}
}

// Name notes: pre-2026-05-04 the second Resolve returned ErrAlreadyAnswered;
// after the atomic-pop refactor it now returns ErrNoPendingQuestion (the
// entry is gone before the second caller can see it). Test name updated to
// reflect the current contract — ErrAlreadyAnswered remains exported only
// for errmap dictionary completeness.
//
// 命名注：2026-05-04 之前第二次 Resolve 返 ErrAlreadyAnswered；原子摘条目
// 重构后改为必返 ErrNoPendingQuestion（条目在第二个调用方能看到之前已被删）。
// 测试名跟着改；ErrAlreadyAnswered 仍导出只为 errmap 字典完整性。
func TestService_Resolve_DoubleAnswerIsErrNoPendingQuestion(t *testing.T) {
	svc := NewService()
	go func() {
		// Hold a Wait open for the test.
		// 让 Wait 持开供测试。
		_, _ = svc.Wait(context.Background(), "call_dup", 2*time.Second)
	}()
	time.Sleep(10 * time.Millisecond)
	if err := svc.Resolve("call_dup", "first"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	// Second one must always be ErrNoPendingQuestion because the first
	// Resolve atomically pops the entry from the registry.
	// 第二次必为 ErrNoPendingQuestion——首次 Resolve 原子地把条目摘走。
	err := svc.Resolve("call_dup", "second")
	if !errors.Is(err, ErrNoPendingQuestion) {
		t.Errorf("want ErrNoPendingQuestion, got %v", err)
	}
}

func TestService_Wait_RejectsDuplicateRegistration(t *testing.T) {
	svc := NewService()
	go func() {
		_, _ = svc.Wait(context.Background(), "call_dup_reg", 2*time.Second)
	}()
	time.Sleep(10 * time.Millisecond)
	_, err := svc.Wait(context.Background(), "call_dup_reg", 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "already pending") {
		t.Errorf("want already-pending error, got %v", err)
	}
}

func TestService_Wait_ManyConcurrent(t *testing.T) {
	svc := NewService()
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			id := "call_" + string(rune('A'+(i%26))) + "_" + string(rune('0'+(i/26)))
			done := make(chan struct{})
			go func() {
				_, _ = svc.Wait(context.Background(), id, 2*time.Second)
				close(done)
			}()
			time.Sleep(5 * time.Millisecond)
			_ = svc.Resolve(id, "ok")
			<-done
		}(i)
	}
	wg.Wait()
	// All entries cleaned.
	// 全部清理。
	if got := svc.pendingCount(); got != 0 {
		t.Errorf("after %d concurrent rounds, %d entries leaked", N, got)
	}
}
