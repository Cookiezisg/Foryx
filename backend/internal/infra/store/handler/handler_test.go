package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database, AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func mkHandler(id, userID, name string) *handlerdomain.Handler {
	return &handlerdomain.Handler{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Description: "test-" + id,
		Tags:        []string{},
	}
}

func mkVersion(id, handlerID, status string) *handlerdomain.Version {
	return &handlerdomain.Version{
		ID:             id,
		HandlerID:      handlerID,
		Status:         status,
		Imports:        "",
		InitBody:       "pass",
		ShutdownBody:   "",
		Methods:        []handlerdomain.MethodSpec{},
		InitArgsSchema: []handlerdomain.InitArgSpec{},
		Dependencies:   []string{},
		PythonVersion:  handlerdomain.DefaultPythonVersion,
		EnvStatus:      handlerdomain.EnvStatusPending,
	}
}

func TestSaveHandler_HappyPath(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	h := mkHandler("hd1", userAlice, "pg-handler")
	if err := s.SaveHandler(ctx, h); err != nil {
		t.Fatalf("SaveHandler: %v", err)
	}

	got, err := s.GetHandler(ctx, "hd1")
	if err != nil {
		t.Fatalf("GetHandler: %v", err)
	}
	if got.Name != "pg-handler" {
		t.Errorf("Name = %q, want pg-handler", got.Name)
	}
}

func TestSaveHandler_DuplicateName(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "shared-name"))
	err := s.SaveHandler(ctx, mkHandler("hd2", userAlice, "shared-name"))
	if !errors.Is(err, handlerdomain.ErrDuplicateName) {
		t.Fatalf("expected ErrDuplicateName, got %v", err)
	}
}

func TestSaveHandler_SoftDeleteAllowsReuse(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "shared-name"))
	if err := s.DeleteHandler(ctx, "hd1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.SaveHandler(ctx, mkHandler("hd2", userAlice, "shared-name")); err != nil {
		t.Errorf("after soft-delete same name should be free: %v", err)
	}
}

func TestSaveHandler_CrossUserIsolation(t *testing.T) {
	s := newStore(t)

	_ = s.SaveHandler(ctxFor(userAlice), mkHandler("hd1", userAlice, "same"))
	if err := s.SaveHandler(ctxFor(userBob), mkHandler("hd2", userBob, "same")); err != nil {
		t.Errorf("cross-user same name should be allowed: %v", err)
	}
	if _, err := s.GetHandler(ctxFor(userAlice), "hd2"); !errors.Is(err, handlerdomain.ErrNotFound) {
		t.Errorf("Alice should NOT see Bob's row; got %v", err)
	}
}

func TestListHandlers_Paginates(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for i := 1; i <= 5; i++ {
		_ = s.SaveHandler(ctx, mkHandler(fmt.Sprintf("hd%d", i), userAlice, fmt.Sprintf("h-%d", i)))
		time.Sleep(2 * time.Millisecond)
	}

	rows, next, err := s.ListHandlers(ctx, handlerdomain.ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("Page 1: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("Page 1 len = %d, want 2", len(rows))
	}
	if next == "" {
		t.Errorf("Page 1 next cursor empty; expected continuation")
	}

	rows2, _, err := s.ListHandlers(ctx, handlerdomain.ListFilter{Cursor: next, Limit: 10})
	if err != nil {
		t.Fatalf("Page 2: %v", err)
	}
	if len(rows2) != 3 {
		t.Errorf("Page 2 len = %d, want 3", len(rows2))
	}
}

func TestSetActiveVersion(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "active-test"))
	if err := s.SetActiveVersion(ctx, "hd1", "hdv-active"); err != nil {
		t.Fatalf("SetActiveVersion: %v", err)
	}
	got, _ := s.GetHandler(ctx, "hd1")
	if got.ActiveVersionID != "hdv-active" {
		t.Errorf("ActiveVersionID = %q, want hdv-active", got.ActiveVersionID)
	}
}

func TestVersionFlow_PendingAcceptReject(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "vf"))

	pending := mkVersion("hdv1", "hd1", handlerdomain.StatusPending)
	if err := s.SaveVersion(ctx, pending); err != nil {
		t.Fatalf("SaveVersion pending: %v", err)
	}

	gotPending, err := s.GetPending(ctx, "hd1")
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if gotPending.ID != "hdv1" {
		t.Errorf("GetPending.ID = %q, want hdv1", gotPending.ID)
	}

	one := 1
	if err := s.UpdateVersionStatus(ctx, "hdv1", handlerdomain.StatusAccepted, &one); err != nil {
		t.Fatalf("UpdateVersionStatus accept: %v", err)
	}
	v, _ := s.GetVersion(ctx, "hdv1")
	if v.Status != handlerdomain.StatusAccepted || v.Version == nil || *v.Version != 1 {
		t.Errorf("after accept: status=%q version=%v, want accepted/1", v.Status, v.Version)
	}

	if _, err := s.GetPending(ctx, "hd1"); !errors.Is(err, handlerdomain.ErrPendingNotFound) {
		t.Errorf("expected ErrPendingNotFound, got %v", err)
	}

	v1, err := s.GetVersionByNumber(ctx, "hd1", 1)
	if err != nil {
		t.Fatalf("GetVersionByNumber: %v", err)
	}
	if v1.ID != "hdv1" {
		t.Errorf("GetVersionByNumber.ID = %q, want hdv1", v1.ID)
	}
}

func TestUpdateVersionEnv(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "ev"))
	_ = s.SaveVersion(ctx, mkVersion("hdv1", "hd1", handlerdomain.StatusAccepted))

	syncedAt := time.Now().UTC()
	if err := s.UpdateVersionEnv(ctx, "hdv1",
		handlerdomain.EnvStatusReady, "", "done", "ok", &syncedAt); err != nil {
		t.Fatalf("UpdateVersionEnv: %v", err)
	}
	v, _ := s.GetVersion(ctx, "hdv1")
	if v.EnvStatus != handlerdomain.EnvStatusReady {
		t.Errorf("EnvStatus = %q, want ready", v.EnvStatus)
	}
}

func TestHardDeleteOldestAccepted_TrimsBeyondCap(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "trim"))
	for i := 1; i <= 7; i++ {
		v := mkVersion(fmt.Sprintf("hdv%d", i), "hd1", handlerdomain.StatusAccepted)
		n := i
		v.Version = &n
		if err := s.SaveVersion(ctx, v); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	if err := s.HardDeleteOldestAccepted(ctx, "hd1", 3); err != nil {
		t.Fatalf("HardDeleteOldestAccepted: %v", err)
	}

	rows, _, _ := s.ListVersions(ctx, "hd1", handlerdomain.VersionListFilter{Limit: 100})
	if len(rows) != 3 {
		t.Errorf("after trim len = %d, want 3", len(rows))
	}
}

func TestUpdateConfigEncrypted_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveHandler(ctx, mkHandler("hd1", userAlice, "cfg"))

	ciphertext := "AES-GCM-fake-bytes-base64"
	if err := s.UpdateConfigEncrypted(ctx, "hd1", ciphertext); err != nil {
		t.Fatalf("UpdateConfigEncrypted: %v", err)
	}
	got, err := s.GetConfigEncrypted(ctx, "hd1")
	if err != nil {
		t.Fatalf("GetConfigEncrypted: %v", err)
	}
	if got != ciphertext {
		t.Errorf("ciphertext mismatch: got %q want %q", got, ciphertext)
	}

	if err := s.ClearConfig(ctx, "hd1"); err != nil {
		t.Fatalf("ClearConfig: %v", err)
	}
	got, _ = s.GetConfigEncrypted(ctx, "hd1")
	if got != "" {
		t.Errorf("after Clear: got %q want \"\"", got)
	}
}

func TestUpdateConfigEncrypted_CrossUserIsolated(t *testing.T) {
	s := newStore(t)

	_ = s.SaveHandler(ctxFor(userAlice), mkHandler("hd1", userAlice, "same"))
	_ = s.UpdateConfigEncrypted(ctxFor(userAlice), "hd1", "alice-ciphertext")

	_, err := s.GetConfigEncrypted(ctxFor(userBob), "hd1")
	if !errors.Is(err, handlerdomain.ErrNotFound) {
		t.Errorf("Bob reading Alice's config should fail; got %v", err)
	}

	err = s.UpdateConfigEncrypted(ctxFor(userBob), "hd1", "bob-ciphertext")
	if !errors.Is(err, handlerdomain.ErrNotFound) {
		t.Errorf("Bob updating Alice's config should fail; got %v", err)
	}

	got, _ := s.GetConfigEncrypted(ctxFor(userAlice), "hd1")
	if got != "alice-ciphertext" {
		t.Errorf("Alice's ciphertext mutated by Bob: got %q", got)
	}
}

func TestGetConfigEncrypted_HandlerNotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetConfigEncrypted(ctxFor(userAlice), "hd-missing")
	if !errors.Is(err, handlerdomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
