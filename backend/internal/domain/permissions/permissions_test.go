// permissions_test.go — domain-layer schema validation.
//
// permissions_test.go ——domain 层 schema 校验。
package permissions

import (
	"errors"
	"testing"
)

func TestIsValidDangerLevel(t *testing.T) {
	for _, l := range []DangerLevel{LevelReadOnly, LevelWorkspaceWrite, LevelDangerFullAccess} {
		if !IsValidDangerLevel(l) {
			t.Errorf("level %q should be valid", l)
		}
	}
	if IsValidDangerLevel("made_up") {
		t.Errorf("made_up should not be valid")
	}
}

func TestIsValidDefaultMode(t *testing.T) {
	for _, m := range []DefaultMode{DefaultModeAsk, DefaultModeAllow, DefaultModeDeny, DefaultModeBypass} {
		if !IsValidDefaultMode(m) {
			t.Errorf("mode %q should be valid", m)
		}
	}
	if IsValidDefaultMode("ridiculous") {
		t.Errorf("ridiculous should not be valid")
	}
}

func TestSettings_EffectiveDefaultMode_FallsBackToAsk(t *testing.T) {
	var s Settings
	if got := s.EffectiveDefaultMode(); got != DefaultModeAsk {
		t.Errorf("empty Settings EffectiveDefaultMode = %q, want %q", got, DefaultModeAsk)
	}
}

func TestSettings_Validate_HappyPath(t *testing.T) {
	s := Settings{
		Permissions: PermissionsBlock{
			DefaultMode: DefaultModeAsk,
			Deny:        []string{"Bash(rm -rf *)"},
			Allow:       []string{"Bash(npm:*)"},
		},
		Hooks: HooksBlock{
			PreToolUse: []HookSpec{{Matcher: "Bash", Command: "/usr/local/bin/guard.sh"}},
		},
	}
	if err := s.Validate(); err != nil {
		t.Errorf("valid settings should pass: %v", err)
	}
}

func TestSettings_Validate_InvalidDefaultMode(t *testing.T) {
	s := Settings{Permissions: PermissionsBlock{DefaultMode: "wild-west"}}
	err := s.Validate()
	if !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("invalid defaultMode should return ErrInvalidSettings, got %v", err)
	}
}

func TestSettings_Validate_EmptyRule(t *testing.T) {
	s := Settings{Permissions: PermissionsBlock{Allow: []string{"Bash(*)", ""}}}
	err := s.Validate()
	if !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("empty rule string should return ErrInvalidSettings, got %v", err)
	}
}

func TestSettings_Validate_HookCommandEmpty(t *testing.T) {
	s := Settings{
		Hooks: HooksBlock{
			PreToolUse: []HookSpec{{Matcher: "Bash", Command: ""}},
		},
	}
	err := s.Validate()
	if !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("empty hook command should return ErrInvalidSettings, got %v", err)
	}
}

func TestSettings_Validate_HookTimeoutNegative(t *testing.T) {
	s := Settings{
		Hooks: HooksBlock{
			PreToolUse: []HookSpec{{Command: "/x", Timeout: -1}},
		},
	}
	err := s.Validate()
	if !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("negative timeout should return ErrInvalidSettings, got %v", err)
	}
}
