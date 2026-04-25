package gotest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestMatchSnapshot_CreatesSnapshotOnFirstRun(t *testing.T) {
	gt := gotest.NewT(t)

	snapDir := filepath.Join("testdata", "__snapshots__")
	t.Cleanup(func() { os.RemoveAll(snapDir) })

	gt.MatchSnapshot("hello world")

	snapName := sanitizeForTest(t.Name()) + ".snap"
	data, err := os.ReadFile(filepath.Join(snapDir, snapName))
	if err != nil {
		t.Fatalf("expected snapshot file to be created: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("snapshot content = %q; want %q", string(data), "hello world")
	}
}

func TestMatchSnapshot_MatchesExistingSnapshot(t *testing.T) {
	gt := gotest.NewT(t)

	snapDir := filepath.Join("testdata", "__snapshots__")
	t.Cleanup(func() { os.RemoveAll(snapDir) })

	// First run creates the snapshot
	gt.MatchSnapshot("stable value")
	// Second run should match without error
	gt.MatchSnapshot("stable value")
}

func TestMatchSnapshot_WithCustomName(t *testing.T) {
	gt := gotest.NewT(t)

	snapDir := filepath.Join("testdata", "__snapshots__")
	t.Cleanup(func() { os.RemoveAll(snapDir) })

	gt.MatchSnapshot("custom content", "my-snapshot")

	snapName := sanitizeForTest(t.Name()) + "_my-snapshot.snap"
	data, err := os.ReadFile(filepath.Join(snapDir, snapName))
	if err != nil {
		t.Fatalf("expected snapshot file to be created: %v", err)
	}
	if string(data) != "custom content" {
		t.Fatalf("snapshot content = %q; want %q", string(data), "custom content")
	}
}

func TestMatchSnapshot_UpdateMode(t *testing.T) {
	gt := gotest.NewT(t)

	snapDir := filepath.Join("testdata", "__snapshots__")
	t.Cleanup(func() { os.RemoveAll(snapDir) })

	// Create initial snapshot
	gt.MatchSnapshot("original")

	// Enable update mode and change value
	t.Setenv("GOTEST_UPDATE_SNAPSHOTS", "1")
	gt.MatchSnapshot("updated")

	snapName := sanitizeForTest(t.Name()) + ".snap"
	data, err := os.ReadFile(filepath.Join(snapDir, snapName))
	if err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}
	if string(data) != "updated" {
		t.Fatalf("snapshot content = %q; want %q", string(data), "updated")
	}
}

func sanitizeForTest(s string) string {
	for _, pair := range [][2]string{{"/", "_"}, {" ", "_"}, {":", "_"}} {
		result := ""
		for _, c := range s {
			if string(c) == pair[0] {
				result += pair[1]
			} else {
				result += string(c)
			}
		}
		s = result
	}
	return s
}
