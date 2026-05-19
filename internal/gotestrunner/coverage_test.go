package gotestrunner //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMergeCoverProfiles(t *testing.T) {
	dir := t.TempDir()

	writeProfile := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	p1 := writeProfile("a.out", "mode: set\nfoo/bar.go:1.2,3.4 1 1\nfoo/bar.go:5.6,7.8 1 0\n")
	p2 := writeProfile("b.out", "mode: set\nfoo/bar.go:5.6,7.8 1 1\nfoo/baz.go:1.2,3.4 1 1\n")

	out := filepath.Join(dir, "merged.out")
	if err := MergeCoverProfiles([]string{p1, p2}, out); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	if lines[0] != "mode: set" {
		t.Errorf("expected mode line, got %q", lines[0])
	}
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (mode + 3 stmts), got %d: %v", len(lines), lines)
	}

	// Verify sorted order: foo/bar.go blocks before foo/baz.go
	if !strings.HasPrefix(lines[1], "foo/bar.go") {
		t.Errorf("line 1 should start with foo/bar.go, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "foo/bar.go") {
		t.Errorf("line 2 should start with foo/bar.go, got %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "foo/baz.go") {
		t.Errorf("line 3 should start with foo/baz.go, got %q", lines[3])
	}

	// Verify max-count aggregation: foo/bar.go:5.6,7.8 should be 1 (max of 0,1)
	if !strings.HasSuffix(lines[2], " 1") {
		t.Errorf("expected max count 1 for overlapping block, got %q", lines[2])
	}
}

func TestMergeCoverProfiles_PreservesUncoveredBlocks(t *testing.T) {
	dir := t.TempDir()

	writeProfile := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Profile A has an uncovered block (count=0) at foo/bar.go:10.1,12.5
	// that does NOT appear in profile B.
	pA := writeProfile("a.out", "mode: set\nfoo/bar.go:1.2,3.4 1 1\nfoo/bar.go:10.1,12.5 1 0\n")
	pB := writeProfile("b.out", "mode: set\nfoo/baz.go:1.2,3.4 1 1\n")

	out := filepath.Join(dir, "merged.out")
	if err := MergeCoverProfiles([]string{pA, pB}, out); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (mode + 3 blocks), got %d: %v", len(lines), lines)
	}

	// Verify the uncovered block is preserved with count 0.
	if !slices.Contains(lines, "foo/bar.go:10.1,12.5 1 0") {
		t.Errorf("uncovered block foo/bar.go:10.1,12.5 with count=0 not found in merged output: %v", lines)
	}
}

func TestMergeCoverProfiles_SkipsMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exists.out")
	os.WriteFile(p, []byte("mode: set\nfoo.go:1.2,3.4 1 1\n"), 0o644)

	out := filepath.Join(dir, "merged.out")
	err := MergeCoverProfiles([]string{filepath.Join(dir, "missing.out"), p}, out)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}
