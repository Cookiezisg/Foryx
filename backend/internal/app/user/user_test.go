package user

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	userstore "github.com/sunweilin/forgify/backend/internal/infra/store/user"
)

func newTestSvc(t *testing.T) *Service {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(db) })
	if err := dbinfra.Migrate(db, &userdomain.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(userstore.New(db), zap.NewNop())
}

func TestEnsureDefault_CreatesOnEmpty(t *testing.T) {
	svc := newTestSvc(t)
	u, err := svc.EnsureDefault(context.Background())
	if err != nil {
		t.Fatalf("EnsureDefault: %v", err)
	}
	if u == nil {
		t.Fatal("expected new default user, got nil")
	}
	if u.ID != "local-user" {
		t.Errorf("ID = %q, want local-user", u.ID)
	}
	if u.Username != "default" {
		t.Errorf("Username = %q, want default", u.Username)
	}
}

func TestEnsureDefault_NoopWhenNonEmpty(t *testing.T) {
	svc := newTestSvc(t)
	_, _ = svc.EnsureDefault(context.Background())
	out, err := svc.EnsureDefault(context.Background())
	if err != nil {
		t.Fatalf("EnsureDefault second call: %v", err)
	}
	if out != nil {
		t.Errorf("second call should return nil, got %+v", out)
	}
}

func TestCreate_UsernameValidation(t *testing.T) {
	svc := newTestSvc(t)
	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"empty", "", userdomain.ErrUsernameRequired},
		{"spaces", "   ", userdomain.ErrUsernameRequired},
		{"uppercase ok (auto-lowercase)", "Alice", nil},
		{"with space", "ali ce", userdomain.ErrUsernameInvalid},
		{"too long", string(make([]byte, 33)), userdomain.ErrUsernameInvalid},
		{"valid", "alice_2", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), CreateInput{Username: tc.input})
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Create(%q): got %v, want %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestCreate_UsernameConflict(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateInput{Username: "alice"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, CreateInput{Username: "alice"})
	if !errors.Is(err, userdomain.ErrUsernameConflict) {
		t.Errorf("got %v, want ErrUsernameConflict", err)
	}
}

func TestDelete_CannotDeleteLast(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	u, _ := svc.Create(ctx, CreateInput{Username: "alice"})
	err := svc.Delete(ctx, u.ID)
	if !errors.Is(err, userdomain.ErrCannotDeleteLast) {
		t.Errorf("got %v, want ErrCannotDeleteLast", err)
	}
}

func TestDelete_NonLastSucceeds(t *testing.T) {
	svc := newTestSvc(t)
	ctx := context.Background()
	_, _ = svc.Create(ctx, CreateInput{Username: "alice"})
	u2, _ := svc.Create(ctx, CreateInput{Username: "bob"})
	if err := svc.Delete(ctx, u2.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, u2.ID); !errors.Is(err, userdomain.ErrNotFound) {
		t.Errorf("post-delete Get should be ErrNotFound, got %v", err)
	}
}
