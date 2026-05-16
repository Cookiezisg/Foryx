package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"go.uber.org/zap"
)

// macCodesign strips com.apple.provenance and ad-hoc-signs the binary; no-op off darwin.
//
// macCodesign 剥 com.apple.provenance 并 ad-hoc 签二进制；非 darwin 直接 no-op。
func macCodesign(ctx context.Context, path string, log *zap.Logger) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	cmd := exec.CommandContext(ctx, "xattr", "-d", "com.apple.provenance", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sandbox.macCodesign: xattr -d: %w (output: %s)", err, out)
	}

	signCmd := exec.CommandContext(ctx, "codesign", "--force", "--sign", "-", path)
	if out, signErr := signCmd.CombinedOutput(); signErr != nil {
		return fmt.Errorf("sandbox.macCodesign %s: %w (output: %s)", path, signErr, out)
	}
	log.Info("mac codesign complete", zap.String("path", path))
	return nil
}
