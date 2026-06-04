// This file tests unexported implementation details (isExternalPackage,
// splitTestName, readAndRestore, pkgCache) and calls MatchSnapshot directly as
// a package-internal function. Exporting these solely for testing would leak
// implementation details into the public API surface. ptest files cannot use
// gotest suites because the suite runner itself lives in this package.
package gotest //nolint:stdlib-test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func thisDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func TestIsExternalPackage(t *testing.T) {
	dir := thisDir()

	t.Run("ptest file returns false", func(t *testing.T) {
		pkgCache.Delete(filepath.Join(dir, "snapshot_internal_test.go"))
		got := isExternalPackage(filepath.Join(dir, "snapshot_internal_test.go"))
		if got {
			t.Fatal("expected false for ptest file")
		}
	})

	t.Run("pxtest file returns true", func(t *testing.T) {
		pkgCache.Delete(filepath.Join(dir, "record_suite_test.go"))
		got := isExternalPackage(filepath.Join(dir, "record_suite_test.go"))
		if !got {
			t.Fatal("expected true for pxtest file")
		}
	})

	t.Run("nonexistent file returns false", func(t *testing.T) {
		got := isExternalPackage(filepath.Join(dir, "nonexistent.go"))
		if got {
			t.Fatal("expected false for nonexistent file")
		}
	})

	t.Run("result is cached", func(t *testing.T) {
		path := filepath.Join(dir, "snapshot_internal_test.go")
		pkgCache.Delete(path)
		isExternalPackage(path)
		_, ok := pkgCache.Load(path)
		if !ok {
			t.Fatal("expected result to be cached")
		}
	})
}

func TestSplitTestName(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantTopLevel string
		wantRest     string
	}{
		{"top-level only", "TestFoo", "TestFoo", ""},
		{"with subtest", "TestFoo/bar", "TestFoo", "bar"},
		{"nested subtests", "TestFoo/bar/baz", "TestFoo", "bar/baz"},
		{"strips dedup suffix", "TestFoo/bar#01", "TestFoo", "bar"},
		{"strips dedup suffix nested", "TestFoo/bar/baz#03", "TestFoo", "bar/baz"},
		{"no dedup suffix", "TestFoo/bar#notnum", "TestFoo", "bar#notnum"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			topLevel, rest := splitTestName(tc.input)
			if topLevel != tc.wantTopLevel {
				t.Errorf("topLevel: want %q, got %q", tc.wantTopLevel, topLevel)
			}
			if rest != tc.wantRest {
				t.Errorf("rest: want %q, got %q", tc.wantRest, rest)
			}
		})
	}
}

func TestMatchSnapshot_PtestUsesNoSuffix(t *testing.T) {
	snapDir := filepath.Join(thisDir(), "testdata", "__snapshots__")
	t.Cleanup(func() { os.Remove(filepath.Join(snapDir, "TestMatchSnapshot_PtestUsesNoSuffix.snap")) })

	MatchSnapshot(t, "ptest-value")

	snapPath := filepath.Join(snapDir, "TestMatchSnapshot_PtestUsesNoSuffix.snap")
	data, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("expected .snap (no _ext suffix): %v", err)
	}
	if !strings.Contains(string(data), "ptest-value") {
		t.Fatal("expected snapshot content")
	}

	extPath := filepath.Join(snapDir, "TestMatchSnapshot_PtestUsesNoSuffix_ext.snap")
	if _, err := os.Stat(extPath); err == nil {
		t.Fatal("_ext.snap should not exist for ptest caller")
	}
}

func TestMatchSnapshot_NormalizesCRLFInContent(t *testing.T) {
	snapDir := filepath.Join(thisDir(), "testdata", "__snapshots__")
	snapFile := filepath.Join(snapDir, "TestMatchSnapshot_NormalizesCRLFInContent.snap")
	t.Cleanup(func() { os.Remove(snapFile) })

	MatchSnapshot(t, "line1\r\nline2\r\n")

	data, err := os.ReadFile(snapFile)
	if err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}
	if strings.Contains(string(data), "\r\n") {
		t.Fatal("snapshot file should not contain \\r\\n — content must be normalized on write")
	}
	if !strings.Contains(string(data), "line1\nline2\n") {
		t.Fatal("expected normalized content in snapshot file")
	}

	MatchSnapshot(t, "line1\r\nline2\r\n")
}

func TestMatchSnapshot_MatchesAfterCRLFFileCorruption(t *testing.T) {
	snapDir := filepath.Join(thisDir(), "testdata", "__snapshots__")
	snapFile := filepath.Join(snapDir, "TestMatchSnapshot_MatchesAfterCRLFFileCorruption.snap")
	t.Cleanup(func() { os.Remove(snapFile) })

	MatchSnapshot(t, "stable value")

	data, err := os.ReadFile(snapFile)
	if err != nil {
		t.Fatalf("read snap: %v", err)
	}
	corrupted := strings.ReplaceAll(string(data), "\n", "\r\n")
	if err := os.WriteFile(snapFile, []byte(corrupted), 0644); err != nil {
		t.Fatalf("write corrupted snap: %v", err)
	}

	MatchSnapshot(t, "stable value")
}

func TestSnapshotReadonly(t *testing.T) {
	t.Run("GOTEST_CI=1 is readonly", func(t *testing.T) {
		t.Setenv("GOTEST_CI", "1")
		if !snapshotReadonly() {
			t.Fatal("expected readonly")
		}
	})
	t.Run("GOTEST_CI=true is readonly", func(t *testing.T) {
		t.Setenv("GOTEST_CI", "true")
		if !snapshotReadonly() {
			t.Fatal("expected readonly")
		}
	})
	t.Run("GOTEST_CI=0 is writable", func(t *testing.T) {
		t.Setenv("GOTEST_CI", "0")
		if snapshotReadonly() {
			t.Fatal("expected writable")
		}
	})
	t.Run("GOTEST_CI unset is writable", func(t *testing.T) {
		t.Setenv("GOTEST_CI", "")
		if snapshotReadonly() {
			t.Fatal("expected writable")
		}
	})
}

func TestMatchSnapshot_CIMode_FailsOnMissingBaseline(t *testing.T) {
	snapDir := filepath.Join(thisDir(), "testdata", "__snapshots__")
	snapFile := filepath.Join(snapDir, "TestMatchSnapshot_CIMode_FailsOnMissingBaseline.snap")
	t.Cleanup(func() { os.Remove(snapFile) })

	t.Setenv("GOTEST_CI", "1")

	mock := &mockT{name: "TestMatchSnapshot_CIMode_FailsOnMissingBaseline/subtest"}
	MatchSnapshot(mock, "new-value")

	if !mock.failed {
		t.Fatal("expected MatchSnapshot to fail in CI mode when no baseline exists")
	}
	if !strings.Contains(mock.msg, "no baseline snapshot") {
		t.Fatalf("expected 'no baseline snapshot' message, got: %s", mock.msg)
	}

	if _, err := os.Stat(snapFile); err == nil {
		t.Fatal("expected snapshot file to NOT be written in CI mode")
	}
}

func TestMatchSnapshot_CIMode_ComparesExistingBaseline(t *testing.T) {
	snapDir := filepath.Join(thisDir(), "testdata", "__snapshots__")
	snapFile := filepath.Join(snapDir, "TestMatchSnapshot_CIMode_ComparesExistingBaseline.snap")
	t.Cleanup(func() { os.Remove(snapFile) })

	MatchSnapshot(t, "expected value")

	t.Setenv("GOTEST_CI", "1")
	MatchSnapshot(t, "expected value")
}

type mockT struct {
	name   string
	failed bool
	msg    string
}

func (m *mockT) Helper()                           {}
func (m *mockT) FailNow()                          {}
func (m *mockT) Name() string                      { return m.name }
func (m *mockT) Errorf(format string, args ...any) { m.failed = true; m.msg = fmt.Sprintf(format, args...) }

func TestReadAndRestore_SeekableReader(t *testing.T) {
	r := strings.NewReader("test data")
	b, err := readAndRestore(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "test data" {
		t.Fatalf("want %q, got %q", "test data", string(b))
	}
	again := Must(io.ReadAll(r))
	if string(again) != "test data" {
		t.Fatalf("reader should be restored; re-read got %q", again)
	}
}

func TestReadAndRestore_NonSeekableReader(t *testing.T) {
	r := io.NopCloser(strings.NewReader("ephemeral"))
	b, err := readAndRestore(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "ephemeral" {
		t.Fatalf("want %q, got %q", "ephemeral", string(b))
	}
	remaining := Must(io.ReadAll(r))
	if len(remaining) != 0 {
		t.Fatal("non-seekable reader should be consumed")
	}
}
