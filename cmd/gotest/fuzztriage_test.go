package main //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func writeCorpusFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "seed")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseCorpusFile_StringWithEscapes(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nstring(\"a@\\x00\")\n")
	args, err := parseCorpusFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0].TypeName != "string" {
		t.Fatalf("expected type string, got %q", args[0].TypeName)
	}
	if args[0].SourceExpr != `"a@\x00"` {
		t.Fatalf("expected verbatim source expr %q, got %q", `"a@\x00"`, args[0].SourceExpr)
	}
}

func TestParseCorpusFile_Int64Negative(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nint64(-3)\n")
	args, err := parseCorpusFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0].TypeName != "int64" || args[0].SourceExpr != "-3" {
		t.Fatalf("unexpected parse result: %+v", args)
	}
}

func TestParseCorpusFile_ByteSlice(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\n[]byte(\"abc\")\n")
	args, err := parseCorpusFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0].TypeName != "[]byte" || args[0].SourceExpr != `"abc"` {
		t.Fatalf("unexpected parse result: %+v", args)
	}
}

func TestParseCorpusFile_MultiArg(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nstring(\"hi\")\nint(5)\nbool(true)\n")
	args, err := parseCorpusFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %+v", len(args), args)
	}
	if args[0].TypeName != "string" || args[1].TypeName != "int" || args[2].TypeName != "bool" {
		t.Fatalf("unexpected types: %+v", args)
	}
	if args[1].SourceExpr != "5" || args[2].SourceExpr != "true" {
		t.Fatalf("unexpected source exprs: %+v", args)
	}
}

func TestParseCorpusFile_MalformedHeader(t *testing.T) {
	path := writeCorpusFile(t, "not a corpus file\nstring(\"hi\")\n")
	_, err := parseCorpusFile(path)
	if err == nil {
		t.Fatal("expected error for malformed header")
	}
}

func TestParseCorpusFile_UnsupportedEntry(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nstruct{X int}{1}\n")
	_, err := parseCorpusFile(path)
	if err == nil {
		t.Fatal("expected error for unsupported corpus entry")
	}
}

func TestCorpusArg_SpliceExpr(t *testing.T) {
	cases := []struct {
		arg  corpusArg
		want string
	}{
		{corpusArg{TypeName: "string", SourceExpr: `"stale"`}, `"stale"`},
		{corpusArg{TypeName: "bool", SourceExpr: "true"}, "true"},
		{corpusArg{TypeName: "int64", SourceExpr: "-3"}, "int64(-3)"},
		{corpusArg{TypeName: "[]byte", SourceExpr: `"abc"`}, `[]byte("abc")`},
	}
	for _, c := range cases {
		if got := c.arg.spliceExpr(); got != c.want {
			t.Errorf("spliceExpr(%+v) = %q, want %q", c.arg, got, c.want)
		}
	}
}

// TestPromoteCrasher_SkipsAndKeepsFileWhenMethodCannotBeEdited exercises the
// confident-skip contract at the promote-crasher-handling level (rather than
// internal/refactor's InsertFuzzAdd directly): when refactor.PromoteFuzzSeed
// refuses to guess at an edit (here: the fuzz method it locates has no
// *gotest.F parameter to identify), promoteCrasher must report failure with
// a "skipped:" message, and — critically — must NOT remove the crasher
// file, since it was never actually promoted into a seed.
//
// This goes through a temp dir + a hand-written fuzztriage.go/fuzzpromote.go
// FuzzTarget rather than the full "gotest fuzz promote" binary + go/packages
// load: a legitimately *discovered* fuzz target (one gotest's own generator
// accepted into overlay.FuzzFuncsByPkg) is, by construction, guaranteed to
// have a real `*gotest.F` parameter — the generator itself rejects any
// FuzzX method that doesn't (see internal/gotestast/spec.go's fuzz-signature
// validation), so this specific confident-skip branch cannot be reached
// through a real, compiling package. Driving promoteCrasher directly is the
// honest way to cover it.
func TestPromoteCrasher_SkipsAndKeepsFileWhenMethodCannotBeEdited(t *testing.T) {
	dir := t.TempDir()
	src := `package example

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(x int) {
}
`
	suitePath := filepath.Join(dir, "suite_test.go")
	if err := os.WriteFile(suitePath, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	corpusPath := filepath.Join(dir, "crasher-seed")
	if err := os.WriteFile(corpusPath, []byte("go test fuzz v1\nstring(\"stale\")\n"), 0600); err != nil {
		t.Fatal(err)
	}

	target := gotestrunner.FuzzTarget{Package: "example.com/foo", Dir: dir, Func: "FuzzFooTestSuite_FuzzTrim"}
	msg, ok := promoteCrasher(target, "FooTestSuite", "FuzzTrim", corpusPath)

	if ok {
		t.Fatalf("expected promoteCrasher to report failure, got ok=true, msg=%q", msg)
	}
	if !strings.Contains(msg, "skipped:") {
		t.Fatalf("expected a %q message, got %q", "skipped:", msg)
	}

	if _, err := os.Stat(corpusPath); err != nil {
		t.Fatalf("expected the crasher file to survive a skipped promote, but os.Stat failed: %v", err)
	}
	got, err := os.ReadFile(suitePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Fatalf("expected the suite source file to be left untouched; got:\n%s", got)
	}
}
