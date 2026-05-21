package gotestrunner //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCleanStaleOverlays_RemovesDeadPID(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
	if err != nil {
		t.Fatal(err)
	}
	// Write a PID that doesn't exist (PID 2 is typically kthreadd on Linux, use a very high PID)
	os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0644)

	CleanStaleOverlays()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		os.RemoveAll(dir)
		t.Fatal("expected stale overlay to be removed")
	}
}

func TestCleanStaleOverlays_KeepsLivePID(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Write our own PID — guaranteed alive
	os.WriteFile(filepath.Join(dir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0644)

	CleanStaleOverlays()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected live overlay to be kept")
	}
}

func TestCleanStaleOverlays_RemovesNoPIDFile(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
	if err != nil {
		t.Fatal(err)
	}

	CleanStaleOverlays()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		os.RemoveAll(dir)
		t.Fatal("expected overlay without PID file to be removed")
	}
}

func TestCleanStaleOverlays_IgnoresNonOverlayDirs(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "unrelated-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	CleanStaleOverlays()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected non-overlay dir to be untouched")
	}
}
