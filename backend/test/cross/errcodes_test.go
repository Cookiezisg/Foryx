//go:build pipeline

// Package cross sweeps sentinel error codes to verify HTTP status + envelope mappings.
//
// Package cross 扫 sentinel 错误码验证 HTTP 状态 + envelope 映射。
package cross

import (
	"net/http"
	"testing"

	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestErrCodes_Sweep(t *testing.T) {
	h := th.New(t)

	var convResp struct {
		Data struct{ ID string `json:"id"` } `json:"data"`
	}
	h.PostJSON("/api/v1/conversations", map[string]any{"title": "errcode-conv"}, &convResp)
	convID := convResp.Data.ID

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		want   int
		code   string
	}{
		{
			"INVALID_REQUEST",
			"POST", "/api/v1/conversations",
			map[string]any{"unknownField": "x"},
			http.StatusBadRequest, "INVALID_REQUEST",
		},

		{
			"NOT_FOUND",
			"GET", "/api/v1/this_route_does_not_exist",
			nil, http.StatusNotFound, "NOT_FOUND",
		},

		{
			"API_KEY_NOT_FOUND",
			"PATCH", "/api/v1/api-keys/aki_doesnotexist000000",
			map[string]any{},
			http.StatusNotFound, "API_KEY_NOT_FOUND",
		},

		{
			"INVALID_PROVIDER",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "alien-provider", "key": "k"},
			http.StatusBadRequest, "INVALID_PROVIDER",
		},

		{
			"BASE_URL_REQUIRED",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "ollama", "key": "k"},
			http.StatusBadRequest, "BASE_URL_REQUIRED",
		},

		{
			"KEY_REQUIRED",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "deepseek"},
			http.StatusBadRequest, "KEY_REQUIRED",
		},

		{
			"API_FORMAT_REQUIRED",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "custom", "key": "k", "baseUrl": "http://localhost"},
			http.StatusBadRequest, "API_FORMAT_REQUIRED",
		},

		{
			"INVALID_SCENARIO",
			"PUT", "/api/v1/model-configs/not-a-real-scenario",
			map[string]any{"provider": "deepseek", "modelId": "x"},
			http.StatusBadRequest, "INVALID_SCENARIO",
		},

		{
			"PROVIDER_REQUIRED",
			"PUT", "/api/v1/model-configs/chat",
			map[string]any{"provider": "", "modelId": "deepseek-chat"},
			http.StatusBadRequest, "PROVIDER_REQUIRED",
		},

		{
			"MODEL_ID_REQUIRED",
			"PUT", "/api/v1/model-configs/chat",
			map[string]any{"provider": "deepseek", "modelId": ""},
			http.StatusBadRequest, "MODEL_ID_REQUIRED",
		},

		{
			"CONVERSATION_NOT_FOUND",
			"PATCH", "/api/v1/conversations/cv_doesnotexist000000",
			map[string]any{"title": "x"},
			http.StatusNotFound, "CONVERSATION_NOT_FOUND",
		},

		{
			"STREAM_NOT_FOUND",
			"DELETE", "/api/v1/conversations/" + convID + "/stream",
			nil, http.StatusNotFound, "STREAM_NOT_FOUND",
		},

	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var errResp th.ErrEnvelope
			status := th.DoRequest(t, h, tc.method, tc.path, tc.body, &errResp)
			if status != tc.want {
				t.Errorf("status=%d, want %d; body=%+v", status, tc.want, errResp)
			}
			if errResp.Error.Code != tc.code {
				t.Errorf("error.code=%q, want %q", errResp.Error.Code, tc.code)
			}
		})
	}
}

func TestErrCodes_FunctionDomain(t *testing.T) {
	h := th.New(t)

	h.NewFunction(t, "errcodes_fn_a", th.SimpleFunctionCode)

	t.Run("FUNCTION_NAME_DUPLICATE", func(t *testing.T) {
		var errResp th.ErrEnvelope
		status := th.PostFunction(t, h, "errcodes_fn_a", th.SimpleFunctionCode, &errResp)
		if status != http.StatusConflict {
			t.Errorf("status=%d, want 409", status)
		}
		if errResp.Error.Code != "FUNCTION_NAME_DUPLICATE" {
			t.Errorf("error.code=%q, want FUNCTION_NAME_DUPLICATE", errResp.Error.Code)
		}
	})

	t.Run("FUNCTION_NOT_FOUND_on_get", func(t *testing.T) {
		var errResp th.ErrEnvelope
		status := th.DoRequest(t, h, "GET", "/api/v1/functions/fn_doesnotexist0000", nil, &errResp)
		if status != http.StatusNotFound {
			t.Errorf("status=%d, want 404", status)
		}
		if errResp.Error.Code != "FUNCTION_NOT_FOUND" {
			t.Errorf("error.code=%q, want FUNCTION_NOT_FOUND", errResp.Error.Code)
		}
	})

	t.Run("FUNCTION_PENDING_NOT_FOUND_on_accept", func(t *testing.T) {
		var createResp struct {
			Data struct {
				Function struct{ ID string `json:"id"` } `json:"function"`
			} `json:"data"`
		}
		th.PostFunction(t, h, "errcodes_fn_b", th.SimpleFunctionCode, &createResp)
		fnID := createResp.Data.Function.ID

		var errResp th.ErrEnvelope
		status := th.DoRequest(t, h, "POST",
			"/api/v1/functions/"+fnID+"/pending:accept", nil, &errResp)
		if status != http.StatusNotFound {
			t.Errorf("status=%d, want 404", status)
		}
		if errResp.Error.Code != "FUNCTION_PENDING_NOT_FOUND" {
			t.Errorf("error.code=%q, want FUNCTION_PENDING_NOT_FOUND", errResp.Error.Code)
		}
	})
}
