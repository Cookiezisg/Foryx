package llmclient_test

import (
	"context"
	"errors"
	"testing"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
)

// fakePicker returns canned (apiKeyID, modelID) per scenario.
// Note: ModelRef now uses APIKeyID instead of Provider.
type fakePicker struct {
	dialogue, utility, agent *modeldomain.ModelRef
}

func (f *fakePicker) PickForDialogue(ctx context.Context) (string, string, error) {
	if f.dialogue == nil {
		return "", "", modeldomain.ErrNotConfigured
	}
	return f.dialogue.APIKeyID, f.dialogue.ModelID, nil
}
func (f *fakePicker) PickForUtility(ctx context.Context) (string, string, error) {
	if f.utility == nil {
		return "", "", modeldomain.ErrNotConfigured
	}
	return f.utility.APIKeyID, f.utility.ModelID, nil
}
func (f *fakePicker) PickForAgent(ctx context.Context) (string, string, error) {
	if f.agent == nil {
		return "", "", modeldomain.ErrNotConfigured
	}
	return f.agent.APIKeyID, f.agent.ModelID, nil
}

// fakeKeys returns canned credentials keyed by api_key id.
type fakeKeys struct {
	byID map[string]apikeydomain.Credentials
}

func (k *fakeKeys) ResolveCredentialsByID(ctx context.Context, apiKeyID string) (apikeydomain.Credentials, error) {
	if c, ok := k.byID[apiKeyID]; ok {
		return c, nil
	}
	return apikeydomain.Credentials{}, apikeydomain.ErrNotFound
}
func (k *fakeKeys) ResolveCredentials(ctx context.Context, provider string) (apikeydomain.Credentials, error) {
	return apikeydomain.Credentials{}, apikeydomain.ErrNotFoundForProvider
}
func (k *fakeKeys) HasKeyForProvider(ctx context.Context, provider string) (bool, error) {
	return false, nil
}
func (k *fakeKeys) MarkInvalid(ctx context.Context, provider string, reason string) error {
	return nil
}
func (k *fakeKeys) DefaultSearchProvider(ctx context.Context) string { return "" }

func newKeys() *fakeKeys {
	return &fakeKeys{byID: map[string]apikeydomain.Credentials{
		"aki_ant1": {Provider: "anthropic", Key: "sk-fake-ant", BaseURL: ""},
		"aki_ds1":  {Provider: "deepseek", Key: "sk-fake-ds", BaseURL: ""},
		"aki_oai1": {Provider: "openai", Key: "sk-fake-oai", BaseURL: ""},
	}}
}

func newFactory(t *testing.T) *llminfra.Factory {
	t.Helper()
	return llminfra.NewFactory()
}

func TestResolveDialogueWithOverride_NoOverride_UsesPicker(t *testing.T) {
	picker := &fakePicker{dialogue: &modeldomain.ModelRef{APIKeyID: "aki_ant1", ModelID: "sonnet"}}
	b, err := llmclientpkg.ResolveDialogueWithOverride(context.Background(), nil, picker, newKeys(), newFactory(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.APIKeyID != "aki_ant1" || b.ModelID != "sonnet" || b.Provider != "anthropic" {
		t.Fatalf("got (%q,%q,%q)", b.APIKeyID, b.ModelID, b.Provider)
	}
}

func TestResolveDialogueWithOverride_WithOverride_BeatsPicker(t *testing.T) {
	picker := &fakePicker{dialogue: &modeldomain.ModelRef{APIKeyID: "aki_ant1", ModelID: "sonnet"}}
	override := &modeldomain.ModelRef{APIKeyID: "aki_oai1", ModelID: "gpt-4o"}
	b, err := llmclientpkg.ResolveDialogueWithOverride(context.Background(), override, picker, newKeys(), newFactory(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.APIKeyID != "aki_oai1" || b.ModelID != "gpt-4o" {
		t.Fatalf("override ignored, got (%q,%q)", b.APIKeyID, b.ModelID)
	}
}

func TestResolveDialogueWithOverride_PickerErrPickModel(t *testing.T) {
	picker := &fakePicker{}
	_, err := llmclientpkg.ResolveDialogueWithOverride(context.Background(), nil, picker, newKeys(), newFactory(t))
	if !errors.Is(err, llmclientpkg.ErrPickModel) {
		t.Fatalf("want ErrPickModel, got %v", err)
	}
}

func TestResolveUtility(t *testing.T) {
	picker := &fakePicker{utility: &modeldomain.ModelRef{APIKeyID: "aki_ant1", ModelID: "haiku"}}
	b, err := llmclientpkg.ResolveUtility(context.Background(), picker, newKeys(), newFactory(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.APIKeyID != "aki_ant1" || b.ModelID != "haiku" {
		t.Fatalf("got (%q,%q)", b.APIKeyID, b.ModelID)
	}
}

func TestResolveAgentWithOverride_NoOverride_UsesPicker(t *testing.T) {
	picker := &fakePicker{agent: &modeldomain.ModelRef{APIKeyID: "aki_ds1", ModelID: "deepseek-chat"}}
	b, err := llmclientpkg.ResolveAgentWithOverride(context.Background(), nil, picker, newKeys(), newFactory(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.APIKeyID != "aki_ds1" || b.Provider != "deepseek" {
		t.Fatalf("got (%q,%q)", b.APIKeyID, b.Provider)
	}
}

func TestResolveAgentWithOverride_WithOverride_Beats(t *testing.T) {
	picker := &fakePicker{agent: &modeldomain.ModelRef{APIKeyID: "aki_ds1", ModelID: "deepseek-chat"}}
	override := &modeldomain.ModelRef{APIKeyID: "aki_ant1", ModelID: "sonnet"}
	b, err := llmclientpkg.ResolveAgentWithOverride(context.Background(), override, picker, newKeys(), newFactory(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.APIKeyID != "aki_ant1" || b.ModelID != "sonnet" {
		t.Fatalf("override ignored, got (%q,%q)", b.APIKeyID, b.ModelID)
	}
}

func TestResolveDialogueWithOverride_OverrideRefersToMissingKey_ErrResolveCreds(t *testing.T) {
	picker := &fakePicker{dialogue: &modeldomain.ModelRef{APIKeyID: "aki_ant1", ModelID: "sonnet"}}
	override := &modeldomain.ModelRef{APIKeyID: "aki_deleted", ModelID: "gpt-4o"}
	_, err := llmclientpkg.ResolveDialogueWithOverride(context.Background(), override, picker, newKeys(), newFactory(t))
	if !errors.Is(err, llmclientpkg.ErrResolveCreds) {
		t.Fatalf("want ErrResolveCreds, got %v", err)
	}
}
