//go:build pipeline

// errcodes_test.go — sweeps sentinel error codes to verify each maps
// to the correct HTTP status + envelope code via a real HTTP path.
//
// Coverage notes:
//   - LLM_PROVIDER_ERROR:   covered by TestChat_LLMStreamError_StatusError
//   - STREAM_IN_PROGRESS:   covered by TestChatQueue_Full_Returns409
//   - API_KEY_TEST_FAILED:  covered by TestAPIKey_Test_FakeServer_Auth401_Returns422
//   - MODEL_NOT_CONFIGURED: covered by TestChat_MissingModelConfig_ErrorCodePersisted
//   - API_KEY_PROVIDER_NOT_FOUND: covered by TestChat_MissingAPIKey_ErrorCodePersisted
//   - Function codes: in TestErrCodes_FunctionDomain below
//
// errcodes_test.go — 扫描 sentinel 错误码，验证每个通过真实 HTTP 路径
// 映射到正确的 HTTP 状态 + envelope code。
package cross

import (
	"net/http"
	"testing"

	th "github.com/sunweilin/forgify/backend/test/harness"
)

// ── Main sweep (no sandbox required) ─────────────────────────────────────────

func TestErrCodes_Sweep(t *testing.T) {
	h := th.New(t)

	// Pre-create a conversation to use in STREAM_NOT_FOUND test.
	// 预建对话用于 STREAM_NOT_FOUND 测试。
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
		// INVALID_REQUEST — unknown JSON field rejected by DisallowUnknownFields.
		// INVALID_REQUEST — DisallowUnknownFields 拒绝未知字段。
		{
			"INVALID_REQUEST",
			"POST", "/api/v1/conversations",
			map[string]any{"unknownField": "x"},
			http.StatusBadRequest, "INVALID_REQUEST",
		},

		// NOT_FOUND — request to an unregistered route.
		// NOT_FOUND — 请求未注册的路由。
		{
			"NOT_FOUND",
			"GET", "/api/v1/this_route_does_not_exist",
			nil, http.StatusNotFound, "NOT_FOUND",
		},

		// API_KEY_NOT_FOUND — PATCH on a non-existent key id.
		// API_KEY_NOT_FOUND — 对不存在的 key id PATCH。
		{
			"API_KEY_NOT_FOUND",
			"PATCH", "/api/v1/api-keys/aki_doesnotexist000000",
			map[string]any{},
			http.StatusNotFound, "API_KEY_NOT_FOUND",
		},

		// INVALID_PROVIDER — unsupported provider name.
		// INVALID_PROVIDER — 不支持的 provider 名。
		{
			"INVALID_PROVIDER",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "alien-provider", "key": "k"},
			http.StatusBadRequest, "INVALID_PROVIDER",
		},

		// BASE_URL_REQUIRED — ollama requires a baseUrl.
		// BASE_URL_REQUIRED — ollama 必须提供 baseUrl。
		{
			"BASE_URL_REQUIRED",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "ollama", "key": "k"},
			http.StatusBadRequest, "BASE_URL_REQUIRED",
		},

		// KEY_REQUIRED — empty key string.
		// KEY_REQUIRED — key 字符串为空。
		{
			"KEY_REQUIRED",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "deepseek"},
			http.StatusBadRequest, "KEY_REQUIRED",
		},

		// API_FORMAT_REQUIRED — custom provider needs apiFormat.
		// API_FORMAT_REQUIRED — custom provider 必须提供 apiFormat。
		{
			"API_FORMAT_REQUIRED",
			"POST", "/api/v1/api-keys",
			map[string]any{"provider": "custom", "key": "k", "baseUrl": "http://localhost"},
			http.StatusBadRequest, "API_FORMAT_REQUIRED",
		},

		// INVALID_SCENARIO — unrecognised model scenario.
		// INVALID_SCENARIO — 未知 model scenario。
		{
			"INVALID_SCENARIO",
			"PUT", "/api/v1/model-configs/not-a-real-scenario",
			map[string]any{"provider": "deepseek", "modelId": "x"},
			http.StatusBadRequest, "INVALID_SCENARIO",
		},

		// PROVIDER_REQUIRED — empty provider in model upsert.
		// PROVIDER_REQUIRED — model upsert provider 为空。
		{
			"PROVIDER_REQUIRED",
			"PUT", "/api/v1/model-configs/chat",
			map[string]any{"provider": "", "modelId": "deepseek-chat"},
			http.StatusBadRequest, "PROVIDER_REQUIRED",
		},

		// MODEL_ID_REQUIRED — empty modelId in model upsert.
		// MODEL_ID_REQUIRED — model upsert modelId 为空。
		{
			"MODEL_ID_REQUIRED",
			"PUT", "/api/v1/model-configs/chat",
			map[string]any{"provider": "deepseek", "modelId": ""},
			http.StatusBadRequest, "MODEL_ID_REQUIRED",
		},

		// CONVERSATION_NOT_FOUND — PATCH non-existent conversation.
		// CONVERSATION_NOT_FOUND — PATCH 不存在的对话。
		{
			"CONVERSATION_NOT_FOUND",
			"PATCH", "/api/v1/conversations/cv_doesnotexist000000",
			map[string]any{"title": "x"},
			http.StatusNotFound, "CONVERSATION_NOT_FOUND",
		},

		// STREAM_NOT_FOUND — DELETE /stream on a conversation with no active stream.
		// STREAM_NOT_FOUND — 对没有活跃流的对话 DELETE /stream。
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

// ── Function errcodes (no sandbox required — pure validation) ───────────────

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
