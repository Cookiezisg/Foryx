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

func TestService_Resolve_DoubleAnswerIsErrNoPendingQuestion(t *testing.T) {
	svc := NewService()
	go func() {
		_, _ = svc.Wait(context.Background(), "call_dup", 2*time.Second)
	}()
	time.Sleep(10 * time.Millisecond)
	if err := svc.Resolve("call_dup", "first"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
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
	if got := svc.pendingCount(); got != 0 {
		t.Errorf("after %d concurrent rounds, %d entries leaked", N, got)
	}
}
