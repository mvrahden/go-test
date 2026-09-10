package main //nolint:stdlib-test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func writeCorpusFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "seed")
	gotest.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func TestParseCorpusFile_StringWithEscapes(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nstring(\"a@\\x00\")\n")
	args, err := parseCorpusFile(path)
	gotest.NoError(t, err)
	gotest.Len(t, args, 1)
	gotest.Equal(t, "string", args[0].TypeName)
	gotest.Equal(t, `"a@\x00"`, args[0].SourceExpr, "expected verbatim source expr")
}

func TestParseCorpusFile_Int64Negative(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nint64(-3)\n")
	args, err := parseCorpusFile(path)
	gotest.NoError(t, err)
	gotest.Len(t, args, 1)
	gotest.Equal(t, "int64", args[0].TypeName)
	gotest.Equal(t, "-3", args[0].SourceExpr)
}

func TestParseCorpusFile_ByteSlice(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\n[]byte(\"abc\")\n")
	args, err := parseCorpusFile(path)
	gotest.NoError(t, err)
	gotest.Len(t, args, 1)
	gotest.Equal(t, "[]byte", args[0].TypeName)
	gotest.Equal(t, `"abc"`, args[0].SourceExpr)
}

func TestParseCorpusFile_MultiArg(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nstring(\"hi\")\nint(5)\nbool(true)\n")
	args, err := parseCorpusFile(path)
	gotest.NoError(t, err)
	gotest.Len(t, args, 3)
	gotest.Equal(t, "string", args[0].TypeName)
	gotest.Equal(t, "int", args[1].TypeName)
	gotest.Equal(t, "bool", args[2].TypeName)
	gotest.Equal(t, "5", args[1].SourceExpr)
	gotest.Equal(t, "true", args[2].SourceExpr)
}

func TestParseCorpusFile_MalformedHeader(t *testing.T) {
	path := writeCorpusFile(t, "not a corpus file\nstring(\"hi\")\n")
	_, err := parseCorpusFile(path)
	gotest.Error(t, err, "expected error for malformed header")
}

func TestParseCorpusFile_UnsupportedEntry(t *testing.T) {
	path := writeCorpusFile(t, "go test fuzz v1\nstruct{X int}{1}\n")
	_, err := parseCorpusFile(path)
	gotest.Error(t, err, "expected error for unsupported corpus entry")
}

func TestExtractDecodedInput(t *testing.T) {
	out := "=== RUN   FuzzX\n" + protocol.FuzzInputPrefix + `Request{Name: "a"}` + "\n--- FAIL: FuzzX\n"
	gotest.Equal(t, `Request{Name: "a"}`, extractDecodedInput(out))
	gotest.Empty(t, extractDecodedInput("no marker here\n"))
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
		gotest.Equal(t, c.want, c.arg.spliceExpr(), "spliceExpr(%+v)", c.arg)
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
	gotest.NoError(t, os.WriteFile(suitePath, []byte(src), 0600))

	corpusPath := filepath.Join(dir, "crasher-seed")
	gotest.NoError(t, os.WriteFile(corpusPath, []byte("go test fuzz v1\nstring(\"stale\")\n"), 0600))

	target := gotestrunner.FuzzTarget{Package: "example.com/foo", Dir: dir, Func: "FuzzFooTestSuite_FuzzTrim"}
	overlay := &gotestrunner.OverlayResult{}
	msg, ok := promoteCrasher(overlay, target, "FooTestSuite", "FuzzTrim", corpusPath)

	gotest.False(t, ok, "expected promoteCrasher to report failure, msg=%q", msg)
	gotest.Contains(t, msg, "skipped:")

	_, err := os.Stat(corpusPath)
	gotest.NoError(t, err, "expected the crasher file to survive a skipped promote")
	got, err := os.ReadFile(suitePath)
	gotest.NoError(t, err)
	gotest.Equal(t, src, string(got), "expected the suite source file to be left untouched")
}

func TestExtractCause_ReportsTheDiagnosticUnderTheFailHeader(t *testing.T) {
	out := "=== RUN   FuzzX\n--- FAIL: FuzzX (0.13s)\n    --- FAIL: FuzzX (0.00s)\n        codec_test.go:12: magic input reached: \"ab\"\n    \n    Failing input written to testdata/fuzz/FuzzX/abc\nFAIL\n"
	gotest.Equal(t, `codec_test.go:12: magic input reached: "ab"`, extractCause(out))
}

func TestExtractCause_KeepsThePanicLine(t *testing.T) {
	out := "--- FAIL: FuzzX (0.00s)\npanic: runtime error: index out of range [3] with length 2\n"
	gotest.Equal(t, "panic: runtime error: index out of range [3] with length 2", extractCause(out))
}

func TestExtractCause_FallsBackToTheFailHeader(t *testing.T) {
	gotest.Equal(t, "--- FAIL: FuzzX (0.00s)", extractCause("--- FAIL: FuzzX (0.00s)\nFAIL\n"))
	gotest.Contains(t, extractCause("nothing useful\n"), "no panic/FAIL line found")
}

func TestExtractCause_VerboseOrderPutsDiagnosticBeforeHeaders(t *testing.T) {
	// Under -v the diagnostic prints while the subtest runs, ahead of the
	// FAIL headers that close it; the cause is still the diagnostic.
	out := strings.Join([]string{
		"=== RUN   FuzzX",
		"=== RUN   FuzzX/771e938e",
		`    suite_test.go:11: unexpected input "0"`,
		"--- FAIL: FuzzX (0.00s)",
		"    --- FAIL: FuzzX/771e938e (0.00s)",
		"FAIL",
		"exit status 1",
	}, "\n")
	gotest.Equal(t, `suite_test.go:11: unexpected input "0"`, extractCause(out))
}
