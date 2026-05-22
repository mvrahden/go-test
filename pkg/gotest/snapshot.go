package gotest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

func matchSnapshot(t testing.TB, callerSkip int, value any, name ...string) {
	t.Helper()

	_, callerFile, _, ok := runtime.Caller(callerSkip)
	if !ok {
		t.Fatalf("MatchSnapshot: unable to determine caller file")
		return
	}

	content := fmt.Sprintf("%v", value)
	testName := t.Name()

	snapName := sanitizeFilename(testName)
	if len(name) > 0 && name[0] != "" {
		snapName += "_" + sanitizeFilename(name[0])
	}
	snapDir := filepath.Join(filepath.Dir(callerFile), "testdata", "__snapshots__")
	snapPath := filepath.Join(snapDir, snapName+".snap")

	if os.Getenv("GOTEST_UPDATE_SNAPSHOTS") != "" {
		if err := os.MkdirAll(snapDir, 0755); err != nil {
			t.Fatalf("MatchSnapshot: failed to create snapshot dir: %v", err)
			return
		}
		if err := os.WriteFile(snapPath, []byte(content), 0644); err != nil {
			t.Fatalf("MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		t.Logf("updated snapshot: %s", snapPath)
		return
	}

	existing, err := os.ReadFile(snapPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(snapDir, 0755); err != nil {
				t.Fatalf("MatchSnapshot: failed to create snapshot dir: %v", err)
				return
			}
			if err := os.WriteFile(snapPath, []byte(content), 0644); err != nil {
				t.Fatalf("MatchSnapshot: failed to write snapshot: %v", err)
				return
			}
			t.Logf("created snapshot: %s", snapPath)
			return
		}
		t.Fatalf("MatchSnapshot: failed to read snapshot: %v", err)
		return
	}

	want := string(existing)
	if content != want {
		d := assert.Diff(want, content)
		if d != "" {
			t.Fatalf("MatchSnapshot: snapshot mismatch (%s):\n  diff:\n%s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", snapPath, d)
		} else {
			t.Fatalf("MatchSnapshot: snapshot mismatch (%s):\n  expected: %s\n  actual:   %s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", snapPath, want, content)
		}
	}
}

func MatchSnapshot(t testing.TB, value any, name ...string) {
	t.Helper()
	matchSnapshot(t, 2, value, name...)
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}
