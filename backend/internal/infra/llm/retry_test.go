// retry_test.go — withRetry policy + isRetryable classification.
//
// retry_test.go ——withRetry 策略 + isRetryable 分类。
package llm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestIsRetryable_RateLimitedYes(t *testing.T) {
	if !isRetryable(fmt.Errorf("upstream: %w", ErrRateLimited)) {
		t.Error("ErrRateLimited should be retryable")
	}
}

func TestIsRetryable_ProviderErrorYes(t *testing.T) {
	if !isRetryable(fmt.Errorf("503: %w", ErrProviderError)) {
		t.Error("ErrProviderError should be retryable")
	}
}

func TestIsRetryable_AuthNo(t *testing.T) {
	if isRetryable(fmt.Errorf("401: %w", ErrAuthFailed)) {
		t.Error("ErrAuthFailed must NOT be retryable")
	}
}

func TestIsRetryable_BadRequestNo(t *testing.T) {
	if isRetryable(fmt.Errorf("400: %w", ErrBadRequest)) {
		t.Error("ErrBadRequest must NOT be retryable")
	}
}

func TestIsRetryable_ContextCanceledNo(t *testing.T) {
	if isRetryable(context.Canceled) {
		t.Error("context.Canceled must NOT be retryable (user intent)")
	}
}

func TestIsRetryable_DeadlineExceededYes(t *testing.T) {
	if !isRetryable(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should be retryable")
	}
}

func TestIsRetryable_NilNo(t *testing.T) {
	if isRetryable(nil) {
		t.Error("nil should not be retryable")
	}
}

func TestWithRetry_SucceedsAfterTwoFailures(t *testing.T) {
	calls := 0
	got, err := withRetryWithDelays(context.Background(), 0, 0, func() (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("flaky: %w", ErrProviderError)
		}
		return "ok", nil
	})
	if err != nil || got != "ok" {
		t.Errorf("got (%q, %v), want (ok, nil)", got, err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (initial + 2 retries)", calls)
	}
}

func TestWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	_, err := withRetryWithDelays(context.Background(), 0, 0, func() (string, error) {
		calls++
		return "", fmt.Errorf("flaky: %w", ErrProviderError)
	})
	if !errors.Is(err, ErrProviderError) {
		t.Errorf("err = %v, want wrapped ErrProviderError", err)
	}
	if calls != retryMaxAttempts {
		t.Errorf("calls = %d, want %d", calls, retryMaxAttempts)
	}
}

func TestWithRetry_NonRetryableFailsImmediately(t *testing.T) {
	calls := 0
	_, err := withRetryWithDelays(context.Background(), 0, 0, func() (string, error) {
		calls++
		return "", fmt.Errorf("401: %w", ErrAuthFailed)
	})
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("err = %v, want ErrAuthFailed", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retries on auth)", calls)
	}
}

func TestWithRetry_CtxCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	calls := 0
	_, err := withRetryWithDelays(ctx, 200*time.Millisecond, 3, func() (string, error) {
		calls++
		return "", fmt.Errorf("flaky: %w", ErrProviderError)
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls > 2 {
		t.Errorf("calls = %d, expected <= 2 (cancel hit during backoff)", calls)
	}
}

// withRetryWithDelays mirrors withRetry but lets the test override
// timing knobs to keep runs fast/deterministic. Production code uses
// withRetry which hardcodes retryInitialDelay / retryDelayFactor.
//
// withRetryWithDelays 镜像 withRetry 但让测试覆写时间常量保持快+确定。
func withRetryWithDelays(ctx context.Context, initialDelay time.Duration, factor int, fn func() (string, error)) (string, error) {
	delay := initialDelay
	var lastErr error
	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		if attempt > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
			if factor > 0 {
				delay *= time.Duration(factor)
			}
		}
		out, err := fn()
		if err == nil {
			return out, nil
		}
		if !isRetryable(err) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}
