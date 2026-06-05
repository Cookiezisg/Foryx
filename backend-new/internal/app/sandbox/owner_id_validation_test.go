package sandbox

import (
	"context"
	"errors"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// TestEnsureEnv_RejectsPATHMetaInOwnerID: owner.ID becomes a directory name and
// joins PATH, so separators / shell metachars / whitespace must be rejected
// before any filesystem work.
//
// owner.ID 会成为目录名并进入 PATH，故分隔符 / shell 元字符 / 空白必须在任何文件系统
// 操作前被拒。
func TestEnsureEnv_RejectsPATHMetaInOwnerID(t *testing.T) {
	svc := newSvc(t, "python")
	bad := []string{
		"cv_abc:python", "cv_abc;python", "cv_abc=python",
		"cv abc python", "cv_abc\tpython", "cv_abc\npython", "cv_abc\x00python",
	}
	for _, id := range bad {
		_, err := svc.EnsureEnv(context.Background(),
			sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindConversation, ID: id},
			sandboxdomain.EnvSpec{Runtime: sandboxdomain.RuntimeSpec{Kind: "python"}},
			nil)
		if !errors.Is(err, sandboxdomain.ErrInvalidOwnerID) {
			t.Errorf("ownerID %q: err = %v, want ErrInvalidOwnerID", id, err)
		}
	}
}

// TestEnsureEnv_AcceptsCleanOwnerID: a clean owner id is not rejected as invalid
// (it proceeds through the full fake install path).
//
// 干净的 owner id 不被判为非法（走完整 fake 装机流程）。
func TestEnsureEnv_AcceptsCleanOwnerID(t *testing.T) {
	svc := newSvc(t, "python")
	_, err := svc.EnsureEnv(context.Background(),
		sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindConversation, ID: "cv_abc_python"},
		sandboxdomain.EnvSpec{Runtime: sandboxdomain.RuntimeSpec{Kind: "python"}},
		nil)
	if errors.Is(err, sandboxdomain.ErrInvalidOwnerID) {
		t.Errorf("clean ownerID wrongly rejected: %v", err)
	}
}
