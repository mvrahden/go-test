package gotest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
	"github.com/mvrahden/go-test/pkg/gotest/internal/snapfile"
)

func matchSnapshot(t testing.TB, callerSkip int, value any, name ...string) {
	t.Helper()

	_, callerFile, _, ok := runtime.Caller(callerSkip)
	if !ok {
		t.Fatalf("MatchSnapshot: unable to determine caller file")
		return
	}

	content := fmt.Sprintf("%v", value)
	if err := snapfile.ValidateContent(content); err != nil {
		t.Fatalf("MatchSnapshot: %v", err)
		return
	}

	testName := t.Name()
	topLevel, sectionKey := splitTestName(testName)
	if len(name) > 0 && name[0] != "" {
		if sectionKey != "" {
			sectionKey += " "
		}
		sectionKey += name[0]
	}
	if sectionKey == "" {
		sectionKey = "_"
	}

	snapDir := filepath.Join(filepath.Dir(callerFile), "testdata", "__snapshots__")
	snapPath := filepath.Join(snapDir, topLevel+".snap")

	if err := os.MkdirAll(snapDir, 0755); err != nil {
		t.Fatalf("MatchSnapshot: failed to create snapshot dir: %v", err)
		return
	}

	existing, _ := os.ReadFile(snapPath)
	sections := snapfile.Parse(existing)

	idx := -1
	for i, s := range sections {
		if s.Key == sectionKey {
			idx = i
			break
		}
	}

	if os.Getenv("GOTEST_UPDATE_SNAPSHOTS") != "" {
		if idx >= 0 {
			sections[idx].Content = content + "\n"
		} else {
			sections = append(sections, snapfile.Section{Key: sectionKey, Content: content + "\n"})
		}
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil {
			t.Fatalf("MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		t.Logf("updated snapshot: %s [%s]", snapPath, sectionKey)
		return
	}

	if idx < 0 {
		sections = append(sections, snapfile.Section{Key: sectionKey, Content: content + "\n"})
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil {
			t.Fatalf("MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		t.Logf("created snapshot: %s [%s]", snapPath, sectionKey)
		return
	}

	want := sections[idx].Content
	got := content + "\n"
	if got != want {
		d := assert.Diff(want, got)
		if d != "" {
			t.Fatalf("MatchSnapshot: snapshot mismatch [%s]:\n  diff:\n%s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", sectionKey, d)
		} else {
			t.Fatalf("MatchSnapshot: snapshot mismatch [%s]:\n  expected: %s\n  actual:   %s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", sectionKey, want, got)
		}
	}
}

func splitTestName(name string) (topLevel, rest string) {
	if top, sub, ok := strings.Cut(name, "/"); ok {
		return top, sub
	}
	return name, ""
}

func MatchSnapshot(t testing.TB, value any, name ...string) {
	t.Helper()
	matchSnapshot(t, 2, value, name...)
}
