package response

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
)

func TestStatusForKind(t *testing.T) {
	cases := map[errorspkg.Kind]int{
		errorspkg.KindInvalid:          http.StatusBadRequest,
		errorspkg.KindUnauthorized:     http.StatusUnauthorized,
		errorspkg.KindNotFound:         http.StatusNotFound,
		errorspkg.KindConflict:         http.StatusConflict,
		errorspkg.KindGone:             http.StatusGone,
		errorspkg.KindForbidden:        http.StatusForbidden,
		errorspkg.KindUnprocessable:    http.StatusUnprocessableEntity,
		errorspkg.KindTooLarge:         http.StatusRequestEntityTooLarge,
		errorspkg.KindUnsupportedMedia: http.StatusUnsupportedMediaType,
		errorspkg.KindRateLimited:      http.StatusTooManyRequests,
		errorspkg.KindBadGateway:       http.StatusBadGateway,
		errorspkg.KindUnavailable:      http.StatusServiceUnavailable,
		errorspkg.KindGatewayTimeout:   http.StatusGatewayTimeout,
		errorspkg.KindAccepted:         http.StatusAccepted,
		errorspkg.KindClientClosed:     499,
		errorspkg.KindInternal:         http.StatusInternalServerError,
	}
	for k, want := range cases {
		if got := statusForKind(k); got != want {
			t.Errorf("statusForKind(%v) = %d, want %d", k, got, want)
		}
	}
}

func decodeErr(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	return env.Error.Code, env.Error.Message
}

func TestFromDomainErrorStructured(t *testing.T) {
	w := httptest.NewRecorder()
	FromDomainError(w, zap.NewNop(), errorspkg.New(errorspkg.KindNotFound, "THING_NOT_FOUND", "thing not found"))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if code, msg := decodeErr(t, w.Body.Bytes()); code != "THING_NOT_FOUND" || msg != "thing not found" {
		t.Errorf("envelope = %q / %q", code, msg)
	}
}

func TestFromDomainErrorWrappedStillMaps(t *testing.T) {
	// A fmt-wrapped *Error must still map via errors.As (no special unwrapping needed).
	base := errorspkg.New(errorspkg.KindConflict, "DUP", "duplicate")
	w := httptest.NewRecorder()
	FromDomainError(w, zap.NewNop(), fmt.Errorf("service layer: %w", base))
	if w.Code != http.StatusConflict {
		t.Errorf("wrapped error status = %d, want 409", w.Code)
	}
	if code, _ := decodeErr(t, w.Body.Bytes()); code != "DUP" {
		t.Errorf("wrapped code = %q, want DUP", code)
	}
}

func TestFromDomainErrorContextCanceled(t *testing.T) {
	w := httptest.NewRecorder()
	FromDomainError(w, zap.NewNop(), context.Canceled)
	if w.Code != 499 {
		t.Errorf("context.Canceled status = %d, want 499", w.Code)
	}
	if code, _ := decodeErr(t, w.Body.Bytes()); code != "CLIENT_CLOSED" {
		t.Errorf("code = %q, want CLIENT_CLOSED", code)
	}
}

func TestFromDomainErrorUnknownIs500AndSuppressed(t *testing.T) {
	w := httptest.NewRecorder()
	FromDomainError(w, zap.NewNop(), errors.New("some internal detail that must not leak"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("unknown error status = %d, want 500", w.Code)
	}
	if code, msg := decodeErr(t, w.Body.Bytes()); code != "INTERNAL_ERROR" || strings.Contains(msg, "leak") {
		t.Errorf("internal detail leaked: code=%q msg=%q", code, msg)
	}
}
