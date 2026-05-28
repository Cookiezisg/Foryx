//go:build pipeline

// Package cross — end-to-end validation of the 3-scenario model selection
// design (dialogue / utility / agent). Covers PUT/GET /model-configs,
// GET /scenarios, INVALID_SCENARIO + MODEL_NOT_CONFIGURED + API_KEY_ID_REQUIRED.
//
// Subagent inheritance is intentionally out of scope here; it requires the
// fake LLM to record per-call (apiKeyID, modelID) which would be a non-trivial
// harness extension. The behavior is covered indirectly by unit tests in
// app/skill and app/subagent.
// TODO Task 12-bonus: lift subagent inheritance into a pipeline assertion
// once fake LLM records resolved credentials per call.
//
// Package cross — 3 个 scenario(dialogue/utility/agent)模型选择端到端验证。
// 覆盖 PUT/GET /model-configs、GET /scenarios、INVALID_SCENARIO、
// MODEL_NOT_CONFIGURED、API_KEY_ID_REQUIRED。subagent 继承不在本任务范围,
// 见 app/skill 与 app/subagent 的单测。
package cross

import (
	"net/http"
	"testing"
	"time"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// seedAPIKey mints an api_key for the default test user via the service so
// model.Upsert's F1 (api_key must exist) is satisfied.
//
// seedAPIKey 通过 service 给默认 test user 种 api_key,满足 model.Upsert F1。
func seedAPIKey(t *testing.T, h *th.Harness, provider, displayName string) string {
	t.Helper()
	k, err := h.APIKey.Create(h.LocalCtx(), apikeyapp.CreateInput{
		Provider:    provider,
		DisplayName: displayName,
		Key:         "sk-fake-" + provider,
	})
	if err != nil {
		t.Fatalf("seed apikey %s: %v", provider, err)
	}
	return k.ID
}

// covers: api:PUT /api/v1/model-configs/{scenario}
// covers: api:GET /api/v1/model-configs
func TestModelScenarios_OnboardingWrites3Rows(t *testing.T) {
	h := th.New(t)
	apiKeyID := seedAPIKey(t, h, "anthropic", "anthropic-onboarding")

	body := map[string]any{"apiKeyId": apiKeyID, "modelId": "claude-sonnet-4-5"}
	for _, scenario := range []string{"dialogue", "utility", "agent"} {
		var resp struct {
			Data struct {
				ID       string `json:"id"`
				Scenario string `json:"scenario"`
				APIKeyID string `json:"apiKeyId"`
				ModelID  string `json:"modelId"`
			} `json:"data"`
		}
		status := th.DoRequest(t, h, "PUT", "/api/v1/model-configs/"+scenario, body, &resp)
		if status != http.StatusOK {
			t.Fatalf("PUT %s: status=%d, want 200", scenario, status)
		}
		if resp.Data.Scenario != scenario {
			t.Errorf("PUT %s: scenario=%q, want %q", scenario, resp.Data.Scenario, scenario)
		}
		if resp.Data.APIKeyID != apiKeyID {
			t.Errorf("PUT %s: apiKeyId=%q, want %q", scenario, resp.Data.APIKeyID, apiKeyID)
		}
		if resp.Data.ModelID != "claude-sonnet-4-5" {
			t.Errorf("PUT %s: modelId=%q, want claude-sonnet-4-5", scenario, resp.Data.ModelID)
		}
	}

	var listResp struct {
		Data []struct {
			Scenario string `json:"scenario"`
			APIKeyID string `json:"apiKeyId"`
			ModelID  string `json:"modelId"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/model-configs", &listResp)
	if len(listResp.Data) != 3 {
		t.Fatalf("list: got %d rows, want 3", len(listResp.Data))
	}
	seen := map[string]bool{}
	for _, r := range listResp.Data {
		seen[r.Scenario] = true
		if r.APIKeyID != apiKeyID {
			t.Errorf("list[%s]: apiKeyId=%q, want %q", r.Scenario, r.APIKeyID, apiKeyID)
		}
		if r.ModelID != "claude-sonnet-4-5" {
			t.Errorf("list[%s]: modelId=%q, want claude-sonnet-4-5", r.Scenario, r.ModelID)
		}
	}
	for _, want := range []string{"dialogue", "utility", "agent"} {
		if !seen[want] {
			t.Errorf("list missing scenario %q", want)
		}
	}
}

// covers: api:GET /api/v1/scenarios
func TestModelScenarios_ScenariosEndpointReturns3(t *testing.T) {
	h := th.New(t)

	var resp struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/scenarios", &resp)
	if len(resp.Data) != 3 {
		t.Fatalf("scenarios: got %d rows, want 3", len(resp.Data))
	}
	names := map[string]bool{}
	for _, r := range resp.Data {
		names[r.Name] = true
	}
	for _, want := range []string{"dialogue", "utility", "agent"} {
		if !names[want] {
			t.Errorf("scenarios missing %q (got %v)", want, names)
		}
	}
}

// covers: errcode:INVALID_SCENARIO
func TestModelScenarios_LegacyChatRejected(t *testing.T) {
	h := th.New(t)
	apiKeyID := seedAPIKey(t, h, "anthropic", "legacy-chat-test")

	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "PUT", "/api/v1/model-configs/chat",
		map[string]any{"apiKeyId": apiKeyID, "modelId": "claude-sonnet-4-5"}, &errResp)
	if status != http.StatusBadRequest {
		t.Fatalf("PUT /model-configs/chat: status=%d, want 400", status)
	}
	if errResp.Error.Code != "INVALID_SCENARIO" {
		t.Fatalf("error.code=%q, want INVALID_SCENARIO", errResp.Error.Code)
	}
}

// covers: errcode:MODEL_NOT_CONFIGURED
// covers: cross:model_not_configured_surfaces_in_chat
//
// When dialogue is missing, chat send must surface MODEL_NOT_CONFIGURED via
// the assistant message's terminal error (chat resolves dialogue scenario for
// user-facing send path).
//
// dialogue 缺失时,chat send 必须在 assistant 终态 message 暴露 MODEL_NOT_CONFIGURED。
func TestModelScenarios_MissingDialogueSurfacesInChat(t *testing.T) {
	h := th.New(t)
	apiKeyID := seedAPIKey(t, h, "anthropic", "missing-dialogue-test")

	// Only utility configured; dialogue intentionally missing.
	//
	// 只配 utility,故意不配 dialogue。
	var upsertResp struct{}
	status := th.DoRequest(t, h, "PUT", "/api/v1/model-configs/utility",
		map[string]any{"apiKeyId": apiKeyID, "modelId": "claude-sonnet-4-5"}, &upsertResp)
	if status != http.StatusOK {
		t.Fatalf("setup utility: status=%d, want 200", status)
	}

	conv := h.NewConversation(t, "missing-dialogue")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "anything")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusError {
		t.Fatalf("status=%q, want error\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	if final.ErrorCode != "MODEL_NOT_CONFIGURED" {
		t.Errorf("errorCode=%q, want MODEL_NOT_CONFIGURED", final.ErrorCode)
	}
}

// covers: errcode:API_KEY_ID_REQUIRED
func TestModelScenarios_APIKeyIDRequired(t *testing.T) {
	h := th.New(t)
	seedAPIKey(t, h, "anthropic", "apikey-required-test")

	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "PUT", "/api/v1/model-configs/dialogue",
		map[string]any{"modelId": "claude-sonnet-4-5"}, &errResp)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", status)
	}
	if errResp.Error.Code != "API_KEY_ID_REQUIRED" {
		t.Fatalf("error.code=%q, want API_KEY_ID_REQUIRED", errResp.Error.Code)
	}
}
