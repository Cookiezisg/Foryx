package apikey

import (
	"errors"
	"strings"
	"testing"

	apikeydomain "github.com/sunweilin/anselm/backend/internal/domain/apikey"
)

const anselmModelsBody = `{"object":"list","data":[{"id":"deepseek-v4-flash","object":"model"}]}`

func TestCreateManaged_SeedsOKProbeArchive(t *testing.T) {
	s, repo := newSvc(nil)
	k, err := s.CreateManaged(ctxWS(), ManagedCreateInput{
		Provider:     "anselm",
		DisplayName:  "Anselm Free (DeepSeek)",
		Key:          "gwk_token123456",
		BaseURL:      "https://api.anselm.website/v1",
		TestResponse: anselmModelsBody,
	})
	if err != nil {
		t.Fatalf("CreateManaged: %v", err)
	}
	// Token encrypted + masked, like any credential.
	if k.KeyEncrypted != "ENC:gwk_token123456" {
		t.Errorf("token not encrypted: %q", k.KeyEncrypted)
	}
	if strings.Contains(k.KeyMasked, "token") {
		t.Errorf("token not masked: %q", k.KeyMasked)
	}
	// Probe archive seeded directly — ok + synthetic /models body, no live probe.
	if k.TestStatus != apikeydomain.TestStatusOK {
		t.Errorf("status = %q, want ok", k.TestStatus)
	}
	if k.TestResponse != anselmModelsBody {
		t.Errorf("test_response not seeded: %q", k.TestResponse)
	}
	if k.LastTestedAt == nil {
		t.Error("last_tested_at should be set")
	}
	// The model module reads this archive (ListProbed); it must surface the seeded model so the
	// picker isn't left in the "ok but no selectable model" dead state.
	probed, _ := repo.ListProbed(ctxWS())
	if len(probed) != 1 || probed[0].TestStatus != apikeydomain.TestStatusOK || probed[0].TestResponse != anselmModelsBody {
		t.Errorf("probe archive not visible to model module: %+v", probed)
	}
}

func TestCreateManaged_Validation(t *testing.T) {
	s, _ := newSvc(nil)
	if _, err := s.CreateManaged(ctxWS(), ManagedCreateInput{Provider: "nope", Key: "k"}); !errors.Is(err, apikeydomain.ErrInvalidProvider) {
		t.Errorf("unknown provider → %v, want ErrInvalidProvider", err)
	}
	if _, err := s.CreateManaged(ctxWS(), ManagedCreateInput{Provider: "anselm", Key: " "}); !errors.Is(err, apikeydomain.ErrKeyRequired) {
		t.Errorf("empty token → %v, want ErrKeyRequired", err)
	}
}

func TestUpdate_ManagedImmutable(t *testing.T) {
	s, _ := newSvc(nil)
	k, _ := s.CreateManaged(ctxWS(), ManagedCreateInput{
		Provider: "anselm", DisplayName: "Anselm Free (DeepSeek)", Key: "gwk_x123456789",
		BaseURL: "https://api.anselm.website/v1", TestResponse: anselmModelsBody,
	})
	name := "hacked"
	if _, err := s.Update(ctxWS(), k.ID, UpdateInput{DisplayName: &name}); !errors.Is(err, apikeydomain.ErrManaged) {
		t.Errorf("editing managed key → %v, want ErrManaged", err)
	}
	// A normal (non-managed) key stays editable — the guard must not over-reach.
	ok, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "mine", Key: "sk-1234567890"})
	newName := "renamed"
	if _, err := s.Update(ctxWS(), ok.ID, UpdateInput{DisplayName: &newName}); err != nil {
		t.Errorf("editing normal key should work, got %v", err)
	}
}

func TestAnselmProviderIsManaged(t *testing.T) {
	meta, ok := GetProviderMeta("anselm")
	if !ok || !meta.Managed {
		t.Errorf("anselm meta = %+v, ok=%v; want Managed=true", meta, ok)
	}
	if meta.DefaultBaseURL != "https://api.anselm.website/v1" {
		t.Errorf("anselm base = %q, want the gateway /v1 root", meta.DefaultBaseURL)
	}
	// The only managed provider today — guard against accidentally flagging a user-key provider.
	if m, _ := GetProviderMeta("openai"); m.Managed {
		t.Error("openai must not be managed")
	}
}
