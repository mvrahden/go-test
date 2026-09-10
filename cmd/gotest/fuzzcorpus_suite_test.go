package main_test

import (
	"os"
	"path/filepath"
	"strings"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzCorpusTestSuite covers the pure halves of triage and promote: reading
// a corpus file, splicing its values back into Go source, and picking the
// cause and decoded input out of a failing target's output.
type FuzzCorpusTestSuite struct{}

func (s *FuzzCorpusTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func writeCorpusFile(t *gotest.T, dir, body string) string {
	path := filepath.Join(dir, "seed")
	gotest.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func (s *FuzzCorpusTestSuite) TestParseCorpusFile(t *gotest.T) {
	t.It("keeps a string's escapes verbatim", func(it *gotest.T) {
		path := writeCorpusFile(it, it.TempDir(), "go test fuzz v1\nstring(\"a@\\x00\")\n")
		args, err := main.ExportParseCorpusFile(path)
		gotest.NoError(it, err)
		gotest.Len(it, args, 1)
		gotest.Equal(it, "string", args[0].TypeName)
		gotest.Equal(it, `"a@\x00"`, args[0].SourceExpr)
	})

	t.It("reads a negative int64", func(it *gotest.T) {
		path := writeCorpusFile(it, it.TempDir(), "go test fuzz v1\nint64(-3)\n")
		args, err := main.ExportParseCorpusFile(path)
		gotest.NoError(it, err)
		gotest.Len(it, args, 1)
		gotest.Equal(it, "int64", args[0].TypeName)
		gotest.Equal(it, "-3", args[0].SourceExpr)
	})

	t.It("reads a byte slice", func(it *gotest.T) {
		path := writeCorpusFile(it, it.TempDir(), "go test fuzz v1\n[]byte(\"abc\")\n")
		args, err := main.ExportParseCorpusFile(path)
		gotest.NoError(it, err)
		gotest.Len(it, args, 1)
		gotest.Equal(it, "[]byte", args[0].TypeName)
		gotest.Equal(it, `"abc"`, args[0].SourceExpr)
	})

	t.It("reads every argument of a multi-argument entry in order", func(it *gotest.T) {
		path := writeCorpusFile(it, it.TempDir(), "go test fuzz v1\nstring(\"hi\")\nint(5)\nbool(true)\n")
		args, err := main.ExportParseCorpusFile(path)
		gotest.NoError(it, err)
		gotest.Len(it, args, 3)
		gotest.Equal(it, "string", args[0].TypeName)
		gotest.Equal(it, "int", args[1].TypeName)
		gotest.Equal(it, "bool", args[2].TypeName)
		gotest.Equal(it, "5", args[1].SourceExpr)
		gotest.Equal(it, "true", args[2].SourceExpr)
	})

	t.It("rejects a malformed header", func(it *gotest.T) {
		path := writeCorpusFile(it, it.TempDir(), "not a corpus file\nstring(\"hi\")\n")
		_, err := main.ExportParseCorpusFile(path)
		gotest.Error(it, err)
	})

	t.It("rejects an unsupported entry", func(it *gotest.T) {
		path := writeCorpusFile(it, it.TempDir(), "go test fuzz v1\nstruct{X int}{1}\n")
		_, err := main.ExportParseCorpusFile(path)
		gotest.Error(it, err)
	})
}

func (s *FuzzCorpusTestSuite) TestSpliceExpr(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		arg  main.ExportCorpusArg
		want string
	}{
		{Desc: "string stays a literal", arg: main.ExportCorpusArg{TypeName: "string", SourceExpr: `"stale"`}, want: `"stale"`},
		{Desc: "bool stays a literal", arg: main.ExportCorpusArg{TypeName: "bool", SourceExpr: "true"}, want: "true"},
		{Desc: "int64 is converted", arg: main.ExportCorpusArg{TypeName: "int64", SourceExpr: "-3"}, want: "int64(-3)"},
		{Desc: "[]byte is converted", arg: main.ExportCorpusArg{TypeName: "[]byte", SourceExpr: `"abc"`}, want: `[]byte("abc")`},
	}) {
		gotest.Equal(sub, tc.want, main.ExportSpliceExpr(tc.arg))
	}
}

func (s *FuzzCorpusTestSuite) TestExtractDecodedInput(t *gotest.T) {
	t.It("returns the literal after the input marker", func(it *gotest.T) {
		out := "=== RUN   FuzzX\n" + protocol.FuzzInputPrefix + `Request{Name: "a"}` + "\n--- FAIL: FuzzX\n"
		gotest.Equal(it, `Request{Name: "a"}`, main.ExportExtractDecodedInput(out))
	})

	t.It("is empty without a marker", func(it *gotest.T) {
		gotest.Empty(it, main.ExportExtractDecodedInput("no marker here\n"))
	})
}

func (s *FuzzCorpusTestSuite) TestExtractCause(t *gotest.T) {
	t.It("reports the diagnostic under the FAIL header", func(it *gotest.T) {
		out := "=== RUN   FuzzX\n--- FAIL: FuzzX (0.13s)\n    --- FAIL: FuzzX (0.00s)\n        codec_test.go:12: magic input reached: \"ab\"\n    \n    Failing input written to testdata/fuzz/FuzzX/abc\nFAIL\n"
		gotest.Equal(it, `codec_test.go:12: magic input reached: "ab"`, main.ExportExtractCause(out))
	})

	t.It("keeps the panic line", func(it *gotest.T) {
		out := "--- FAIL: FuzzX (0.00s)\npanic: runtime error: index out of range [3] with length 2\n"
		gotest.Equal(it, "panic: runtime error: index out of range [3] with length 2", main.ExportExtractCause(out))
	})

	t.It("falls back to the FAIL header, then to a note", func(it *gotest.T) {
		gotest.Equal(it, "--- FAIL: FuzzX (0.00s)", main.ExportExtractCause("--- FAIL: FuzzX (0.00s)\nFAIL\n"))
		gotest.Contains(it, main.ExportExtractCause("nothing useful\n"), "no panic/FAIL line found")
	})

	t.It("finds the diagnostic ahead of the headers under -v", func(it *gotest.T) {
		// Under -v the diagnostic prints while the subtest runs, before the
		// FAIL headers that close it.
		out := strings.Join([]string{
			"=== RUN   FuzzX",
			"=== RUN   FuzzX/771e938e",
			`    suite_test.go:11: unexpected input "0"`,
			"--- FAIL: FuzzX (0.00s)",
			"    --- FAIL: FuzzX/771e938e (0.00s)",
			"FAIL",
			"exit status 1",
		}, "\n")
		gotest.Equal(it, `suite_test.go:11: unexpected input "0"`, main.ExportExtractCause(out))
	})
}

// TestPromoteCrasher drives promoteCrasher directly: a discovered fuzz
// target always has a *gotest.F parameter (the generator rejects the rest),
// so the confident-skip branch is unreachable through a real package.
func (s *FuzzCorpusTestSuite) TestPromoteCrasher(t *gotest.T) {
	t.When("the method cannot be edited", func(w *gotest.T) {
		w.It("reports a skip and keeps the crasher file and the source", func(it *gotest.T) {
			dir := it.TempDir()
			src := "package example\n\ntype FooTestSuite struct{}\n\nfunc (s *FooTestSuite) FuzzTrim(x int) {\n}\n"
			suitePath := filepath.Join(dir, "suite_test.go")
			gotest.NoError(it, os.WriteFile(suitePath, []byte(src), 0600))
			corpusPath := filepath.Join(dir, "crasher-seed")
			gotest.NoError(it, os.WriteFile(corpusPath, []byte("go test fuzz v1\nstring(\"stale\")\n"), 0600))

			target := gotestrunner.FuzzTarget{Package: "example.com/foo", Dir: dir, Func: "FuzzFooTestSuite_FuzzTrim"}
			msg, ok := main.ExportPromoteCrasher(&gotestrunner.OverlayResult{}, target, "FooTestSuite", "FuzzTrim", corpusPath)

			gotest.False(it, ok, "msg=%q", msg)
			gotest.Contains(it, msg, "skipped:")
			_, err := os.Stat(corpusPath)
			gotest.NoError(it, err, "the crasher file must survive a skipped promote")
			got, err := os.ReadFile(suitePath)
			gotest.NoError(it, err)
			gotest.Equal(it, src, string(got))
		})
	})
}
