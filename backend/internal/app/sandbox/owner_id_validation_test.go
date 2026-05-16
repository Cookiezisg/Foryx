package sandbox

import (
	"context"
	"strings"
	"testing"

	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	"go.uber.org/zap"
)

func TestEnsureEnv_RejectsPATHMetaCharsInOwnerID(t *testing.T) {
	db, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	if err := db.AutoMigrate(&sandboxdomain.Runtime{}, &sandboxdomain.Env{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := sandboxstore.New(db)
	svc := New(repo, t.TempDir(), nil, zap.NewNop())
	svc.MarkReadyForTest("/fake/mise")

	cases := []struct {
		name     string
		ownerID  string
		wantHint string
	}{
		{"colon", "cv_abc:python", "PATH-meta"},
		{"semicolon", "cv_abc;python", "PATH-meta"},
		{"equals", "cv_abc=python", "PATH-meta"},
		{"space", "cv abc python", "whitespace"},
		{"tab", "cv_abc\tpython", "whitespace"},
		{"newline", "cv_abc\npython", "whitespace"},
		{"null", "cv_abc\x00python", "whitespace"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.EnsureEnv(context.Background(),
				sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindConversation, ID: c.ownerID},
				sandboxdomain.EnvSpec{Runtime: sandboxdomain.RuntimeSpec{Kind: "python"}},
				nil)
			if err == nil {
				t.Fatalf("ownerID %q should be rejected but EnsureEnv returned nil", c.ownerID)
			}
			if !strings.Contains(err.Error(), "PATH-meta") &&
				!strings.Contains(err.Error(), "whitespace") {
				t.Errorf("ownerID %q rejected with unexpected error %q (want hint about PATH-meta or whitespace)",
					c.ownerID, err.Error())
			}
		})
	}
}

func TestEnsureEnv_AcceptsCleanOwnerID(t *testing.T) {
	db, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	if err := db.AutoMigrate(&sandboxdomain.Runtime{}, &sandboxdomain.Env{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := sandboxstore.New(db)
	svc := New(repo, t.TempDir(), nil, zap.NewNop())
	svc.MarkReadyForTest("/fake/mise")

	_, err = svc.EnsureEnv(context.Background(),
		sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindConversation, ID: "cv_abc_python"},
		sandboxdomain.EnvSpec{Runtime: sandboxdomain.RuntimeSpec{Kind: "python"}},
		nil)
	if err == nil {
		t.Skip("EnsureEnv unexpectedly succeeded; this test only checks input-validation path")
	}
	if strings.Contains(err.Error(), "PATH-meta") || strings.Contains(err.Error(), "whitespace") {
		t.Errorf("clean ownerID rejected with PATH-meta/whitespace error: %v", err)
	}
}
