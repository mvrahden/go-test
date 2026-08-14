package refactor //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertFuzzAdd_AfterExistingAdd(t *testing.T) {
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
	})
}
`
	edited, found, err := InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected method to be found")
	}
	out := string(edited)

	wantOrder := []string{`f.Add("  x ")`, `f.Add("stale")`, "gotest.Fuzz(f"}
	lastIdx := -1
	for _, w := range wantOrder {
		idx := strings.Index(out, w)
		if idx < 0 {
			t.Fatalf("expected output to contain %q; got:\n%s", w, out)
		}
		if idx < lastIdx {
			t.Fatalf("expected %q to appear after previous marker; got:\n%s", w, out)
		}
		lastIdx = idx
	}
}

func TestInsertFuzzAdd_AsFirstStatementWhenNoneExist(t *testing.T) {
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {
	})
}
`
	edited, found, err := InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected method to be found")
	}
	out := string(edited)

	addIdx := strings.Index(out, `f.Add("stale")`)
	fuzzIdx := strings.Index(out, "gotest.Fuzz(f")
	if addIdx < 0 {
		t.Fatalf("expected f.Add(\"stale\") in output:\n%s", out)
	}
	if fuzzIdx < 0 || addIdx > fuzzIdx {
		t.Fatalf("expected f.Add to appear before gotest.Fuzz call; got:\n%s", out)
	}
}

func TestInsertFuzzAdd_MultiArg(t *testing.T) {
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzPair(f *gotest.F) {
	gotest.Fuzz2(f, func(t *gotest.T, a string, b int) {
	})
}
`
	edited, found, err := InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzPair", []string{`"stale"`, `int64(-3)`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected method to be found")
	}
	if !strings.Contains(string(edited), `f.Add("stale", int64(-3))`) {
		t.Fatalf("expected multi-arg f.Add call in output:\n%s", edited)
	}
}

func TestInsertFuzzAdd_MethodNotFound(t *testing.T) {
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {})
}
`
	edited, found, err := InsertFuzzAdd([]byte(src), "OtherTestSuite", "FuzzTrim", []string{`"stale"`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected method not to be found")
	}
	if edited != nil {
		t.Fatal("expected no edited output when method is not found")
	}
}

func TestInsertFuzzAdd_ProducesValidGo(t *testing.T) {
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ")
	f.Add("y")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
	})
}
`
	edited, found, err := InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected method to be found")
	}
	// format.Source already validated this parses; additionally check that
	// the new call landed after BOTH existing f.Add calls, not just one.
	out := string(edited)
	idxY := strings.Index(out, `f.Add("y")`)
	idxStale := strings.Index(out, `f.Add("stale")`)
	if idxY < 0 || idxStale < 0 || idxStale < idxY {
		t.Fatalf("expected f.Add(\"stale\") after f.Add(\"y\"); got:\n%s", out)
	}
}

func TestPromoteFuzzSeed_WritesFileAndReturnsLine(t *testing.T) {
	dir := t.TempDir()
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
	})
}
`
	path := filepath.Join(dir, "suite_test.go")
	if err := os.WriteFile(path, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	gotPath, line, err := PromoteFuzzSeed(dir, "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != path {
		t.Fatalf("expected path %q, got %q", path, gotPath)
	}
	if line <= 0 {
		t.Fatalf("expected a positive line number, got %d", line)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `f.Add("stale")`) {
		t.Fatalf("expected f.Add(\"stale\") to be written to disk; got:\n%s", got)
	}
}

// TestInsertFuzzAdd_NoIdentifiableFParam covers the confident-skip safety
// path: the method IS located (found=true) but has no parameter whose
// syntactic type is *gotest.F, so InsertFuzzAdd must refuse to guess which
// parameter to splice an Add call onto rather than silently doing the wrong
// thing — it returns a non-nil error and no edited output.
func TestInsertFuzzAdd_NoIdentifiableFParam(t *testing.T) {
	src := `package example

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(x int) {
}
`
	edited, found, err := InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if !found {
		t.Fatal("expected the method to be found")
	}
	if err == nil {
		t.Fatal("expected an error when no *gotest.F parameter can be identified")
	}
	if edited != nil {
		t.Fatalf("expected no edited output on a confident-skip error, got:\n%s", edited)
	}
}

// TestInsertFuzzAdd_NoBody covers a second confident-skip variant: a
// syntactically valid but bodyless method declaration (legal Go for an
// externally-implemented function/method, e.g. one backed by assembly) —
// decl.Body is nil, so there is nowhere to splice a statement into.
func TestInsertFuzzAdd_NoBody(t *testing.T) {
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F)
`
	edited, found, err := InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if !found {
		t.Fatal("expected the method to be found")
	}
	if err == nil {
		t.Fatal("expected an error for a bodyless method declaration")
	}
	if edited != nil {
		t.Fatalf("expected no edited output on a confident-skip error, got:\n%s", edited)
	}
}

// TestPromoteFuzzSeed_NoIdentifiableFParam_LeavesFileUntouched is the
// PromoteFuzzSeed-level counterpart of TestInsertFuzzAdd_NoIdentifiableFParam:
// once InsertFuzzAdd reports found=true with an error, PromoteFuzzSeed must
// propagate the error and must NOT have written anything to disk.
func TestPromoteFuzzSeed_NoIdentifiableFParam_LeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	src := `package example

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(x int) {
}
`
	path := filepath.Join(dir, "suite_test.go")
	if err := os.WriteFile(path, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := PromoteFuzzSeed(dir, "FooTestSuite", "FuzzTrim", []string{`"stale"`})
	if err == nil {
		t.Fatal("expected an error when no *gotest.F parameter can be identified")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Fatalf("expected file to be left untouched on a confident-skip error; got:\n%s", got)
	}
}

func TestPromoteFuzzSeed_MethodNotFound(t *testing.T) {
	dir := t.TempDir()
	src := `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {})
}
`
	path := filepath.Join(dir, "suite_test.go")
	if err := os.WriteFile(path, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := PromoteFuzzSeed(dir, "FooTestSuite", "FuzzMissing", []string{`"stale"`})
	if err == nil {
		t.Fatal("expected error for missing method")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Fatalf("expected file to be left untouched when method is not found; got:\n%s", got)
	}
}
