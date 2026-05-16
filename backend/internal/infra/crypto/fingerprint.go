// Package crypto implements domain/crypto.Encryptor (AES-GCM keyed by machine fingerprint).
//
// Package crypto 实现 domain/crypto.Encryptor（AES-GCM，密钥从机器指纹派生）。
package crypto

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ErrNoFingerprint signals no stable machine identity; callers MUST refuse to proceed.
//
// ErrNoFingerprint 表示拿不到稳定机器标识，调用方必须拒绝继续。
var ErrNoFingerprint = errors.New("cannot determine machine fingerprint")

// MachineFingerprint returns a stable per-machine ID for key derivation; never returns a fallback.
//
// MachineFingerprint 返回稳定的机器标识用于派生密钥；永不返回 fallback。
func MachineFingerprint() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return fingerprintDarwin()
	case "windows":
		return fingerprintWindows()
	default:
		return fingerprintLinux()
	}
}

func fingerprintDarwin() (string, error) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", fmt.Errorf("%w: ioreg failed: %v", ErrNoFingerprint, err)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.Contains(line, "IOPlatformSerialNumber") {
			continue
		}
		parts := strings.Split(line, "\"")
		if len(parts) >= 4 && parts[3] != "" {
			return parts[3], nil
		}
	}
	return "", fmt.Errorf("%w: IOPlatformSerialNumber not found in ioreg output", ErrNoFingerprint)
}

func fingerprintWindows() (string, error) {
	out, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
	if err != nil {
		return "", fmt.Errorf("%w: reg query failed: %v", ErrNoFingerprint, err)
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: empty reg query output", ErrNoFingerprint)
	}
	guid := parts[len(parts)-1]
	if guid == "" || guid == "MachineGuid" {
		return "", fmt.Errorf("%w: MachineGuid value missing", ErrNoFingerprint)
	}
	return guid, nil
}

func fingerprintLinux() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("%w: read /etc/machine-id: %v", ErrNoFingerprint, err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("%w: /etc/machine-id is empty", ErrNoFingerprint)
	}
	return id, nil
}
