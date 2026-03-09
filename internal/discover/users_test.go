package discover

import (
	"os/user"
	"testing"
)

func TestDetectProjectOwner(t *testing.T) {
	// Create a temp dir which will be owned by the current user
	dir := t.TempDir()

	owner, err := DetectProjectOwner(dir)
	if err != nil {
		t.Fatalf("DetectProjectOwner(%s): %v", dir, err)
	}

	// Should be the current user
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("getting current user: %v", err)
	}

	if owner != currentUser.Username {
		t.Errorf("expected owner %q, got %q", currentUser.Username, owner)
	}
}

func TestDetectProjectOwnerNonexistentPath(t *testing.T) {
	_, err := DetectProjectOwner("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestDetectProjectOwnerReturnsNonEmpty(t *testing.T) {
	dir := t.TempDir()

	owner, err := DetectProjectOwner(dir)
	if err != nil {
		t.Fatalf("DetectProjectOwner: %v", err)
	}

	if owner == "" {
		t.Error("expected non-empty owner string")
	}
}
