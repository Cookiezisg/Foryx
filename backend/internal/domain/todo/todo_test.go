package todo

import (
	"slices"
	"testing"
)

func TestIsValidStatus_KnownStatuses(t *testing.T) {
	for _, s := range []string{StatusPending, StatusInProgress, StatusCompleted, StatusDeleted} {
		if !IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = false, want true", s)
		}
	}
}

func TestIsValidStatus_RejectsUnknown(t *testing.T) {
	for _, s := range []string{"", "PENDING", "done", "wip"} {
		if IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = true, want false", s)
		}
	}
}

func TestListStatuses_NotEmpty(t *testing.T) {
	got := ListStatuses()
	if len(got) == 0 {
		t.Fatal("ListStatuses() returned empty slice")
	}
	if !slices.Contains(got, StatusPending) {
		t.Errorf("ListStatuses missing %q", StatusPending)
	}
}

func TestListStatuses_AllPassIsValid(t *testing.T) {
	for _, s := range ListStatuses() {
		if !IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = false but it is in ListStatuses()", s)
		}
	}
}
