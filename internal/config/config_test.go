package config //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_FullConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	writeFile(t, dir, FileName, `
tags: "integration,e2e"
setup-timeout: 2m
timeout: 20m
min-coverage: 80
parallel: 12
compile-parallel: 2
debounce: 500ms
lint:
  skip:
    - stdlib-test
    - testify
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, "tags", cfg.Tags, "integration,e2e")
	assertDuration(t, "setup-timeout", cfg.SetupTimeout, 2*time.Minute)
	assertDuration(t, "timeout", cfg.Timeout, 20*time.Minute)
	assertEqual(t, "min-coverage", cfg.MinCoverage, 80)
	assertEqual(t, "parallel", cfg.Parallel, 12)
	assertEqual(t, "compile-parallel", cfg.CompileParallel, 2)
	assertDuration(t, "debounce", cfg.Debounce, 500*time.Millisecond)
	assertSliceEqual(t, "lint.skip", cfg.Lint.Skip, []string{"stdlib-test", "testify"})
}

func TestLoad_NoFile_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, "tags", cfg.Tags, "")
	assertNilDuration(t, "setup-timeout", cfg.SetupTimeout)
	assertNilDuration(t, "timeout", cfg.Timeout)
	assertEqual(t, "min-coverage", cfg.MinCoverage, 0)
	assertEqual(t, "parallel", cfg.Parallel, 0)
	assertEqual(t, "compile-parallel", cfg.CompileParallel, 0)
	assertNilDuration(t, "debounce", cfg.Debounce)
	if len(cfg.Lint.Skip) != 0 {
		t.Errorf("lint.skip: got %v, want empty", cfg.Lint.Skip)
	}
}

func TestLoad_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	writeFile(t, dir, FileName, `
tags: "unit"
min-coverage: 60
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, "tags", cfg.Tags, "unit")
	assertEqual(t, "min-coverage", cfg.MinCoverage, 60)
	assertNilDuration(t, "setup-timeout", cfg.SetupTimeout)
	assertNilDuration(t, "debounce", cfg.Debounce)
}

func TestLoad_WalksUpToGoMod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module test\n")
	writeFile(t, root, FileName, `tags: "found"`)

	sub := filepath.Join(root, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, "tags", cfg.Tags, "found")
}

func TestLoad_StopsAtGoMod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, FileName, `tags: "should-not-find"`)

	sub := filepath.Join(root, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "go.mod", "module nested\n")

	cfg, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}

	assertEqual(t, "tags", cfg.Tags, "")
}

func TestLoad_ZeroDuration_IsNotNil(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	writeFile(t, dir, FileName, `
setup-timeout: 0s
timeout: 0s
debounce: 0s
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	assertDuration(t, "setup-timeout", cfg.SetupTimeout, 0)
	assertDuration(t, "timeout", cfg.Timeout, 0)
	assertDuration(t, "debounce", cfg.Debounce, 0)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	writeFile(t, dir, FileName, `{{{invalid`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	writeFile(t, dir, FileName, `setup-timeout: "not-a-duration"`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", field, got, want)
	}
}

func assertDuration(t *testing.T, field string, got *Duration, want time.Duration) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: got nil, want %v", field, want)
		return
	}
	if got.Duration() != want {
		t.Errorf("%s: got %v, want %v", field, got.Duration(), want)
	}
}

func assertNilDuration(t *testing.T, field string, got *Duration) {
	t.Helper()
	if got != nil {
		t.Errorf("%s: got %v, want nil", field, got.Duration())
	}
}

func assertSliceEqual(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v, want %v", field, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: got %q, want %q", field, i, got[i], want[i])
		}
	}
}
