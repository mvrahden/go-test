package refactor_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/refactor"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzPromoteTestSuite covers splicing a seed into a fuzz method: the new
// f.Add lands after the existing ones and before f.Fuzz, and any shape the
// editor cannot place confidently is refused without touching the file.
type FuzzPromoteTestSuite struct{}

func (s *FuzzPromoteTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type promoteCtx struct{ dir string }

func (s *FuzzPromoteTestSuite) BeforeEach(t *gotest.T) *promoteCtx {
	return &promoteCtx{dir: t.TempDir()}
}

const fuzzTrimOneSeed = `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ")
	f.Fuzz(func(t *gotest.T, in string) {
	})
}
`

const fuzzTrimNoSeed = `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in string) {})
}
`

// fuzzTrimNoFParam is found by name but has no *gotest.F to splice onto.
const fuzzTrimNoFParam = `package example

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(x int) {
}
`

func (s *FuzzPromoteTestSuite) writeSuite(t *gotest.T, ctx *promoteCtx, src string) string {
	path := filepath.Join(ctx.dir, "suite_test.go")
	gotest.NoError(t, os.WriteFile(path, []byte(src), 0600))
	return path
}

func (s *FuzzPromoteTestSuite) readSuite(t *gotest.T, path string) string {
	got, err := os.ReadFile(path)
	gotest.NoError(t, err)
	return string(got)
}

func (s *FuzzPromoteTestSuite) TestInsertFuzzAdd(t *gotest.T, _ *promoteCtx) {
	t.It("appends after the existing f.Add and before f.Fuzz", func(it *gotest.T) {
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimOneSeed), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
		gotest.NoError(it, err)
		gotest.True(it, found)
		out := string(edited)
		existing := strings.Index(out, `f.Add("  x ")`)
		added := strings.Index(out, `f.Add("stale")`)
		fuzz := strings.Index(out, "f.Fuzz(")
		gotest.GreaterOrEqual(it, existing, 0, "output:\n%s", out)
		gotest.Greater(it, added, existing, "output:\n%s", out)
		gotest.Greater(it, fuzz, added, "output:\n%s", out)
	})

	t.It("becomes the first statement when no seed exists", func(it *gotest.T) {
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimNoSeed), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
		gotest.NoError(it, err)
		gotest.True(it, found)
		out := string(edited)
		added := strings.Index(out, `f.Add("stale")`)
		fuzz := strings.Index(out, "f.Fuzz(")
		gotest.GreaterOrEqual(it, added, 0, "output:\n%s", out)
		gotest.Greater(it, fuzz, added, "output:\n%s", out)
	})

	t.It("splices every argument of a multi-argument seed", func(it *gotest.T) {
		src := strings.Replace(fuzzTrimNoSeed, "FuzzTrim(f *gotest.F) {\n\tf.Fuzz(func(t *gotest.T, in string) {})", "FuzzPair(f *gotest.F) {\n\tf.Fuzz(func(t *gotest.T, a string, b int) {})", 1)
		edited, found, err := refactor.InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzPair", []string{`"stale"`, `int64(-3)`})
		gotest.NoError(it, err)
		gotest.True(it, found)
		gotest.Contains(it, string(edited), `f.Add("stale", int64(-3))`)
	})

	t.It("lands after every existing f.Add, not only the first", func(it *gotest.T) {
		src := strings.Replace(fuzzTrimOneSeed, "\tf.Add(\"  x \")\n", "\tf.Add(\"  x \")\n\tf.Add(\"y\")\n", 1)
		edited, found, err := refactor.InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
		gotest.NoError(it, err)
		gotest.True(it, found)
		out := string(edited)
		gotest.Greater(it, strings.Index(out, `f.Add("stale")`), strings.Index(out, `f.Add("y")`), "output:\n%s", out)
	})

	t.When("the suite is not in the file", func(w *gotest.T) {
		w.It("reports not found without an edit", func(it *gotest.T) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimNoSeed), "OtherTestSuite", "FuzzTrim", []string{`"stale"`})
			gotest.NoError(it, err)
			gotest.False(it, found)
			gotest.Nil(it, edited)
		})
	})

	t.When("the method has no *gotest.F parameter", func(w *gotest.T) {
		w.It("is found but refuses to guess", func(it *gotest.T) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimNoFParam), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
			gotest.True(it, found)
			gotest.Error(it, err)
			gotest.Nil(it, edited)
		})
	})

	t.When("the method has no body", func(w *gotest.T) {
		w.It("is found but refuses to guess", func(it *gotest.T) {
			// Legal Go for an externally implemented method; nowhere to splice.
			src := "package example\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FooTestSuite struct{}\n\nfunc (s *FooTestSuite) FuzzTrim(f *gotest.F)\n"
			edited, found, err := refactor.InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
			gotest.True(it, found)
			gotest.Error(it, err)
			gotest.Nil(it, edited)
		})
	})
}

const fuzzTrimTrailingComment = `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ") // note
	f.Fuzz(func(t *gotest.T, in string) {
	})
}
`

const fuzzScale = `package example

import "github.com/mvrahden/go-test/pkg/gotest"

type FooTestSuite struct{}

func (s *FooTestSuite) FuzzScale(f *gotest.F) {
	f.Add(1.5)
	f.Fuzz(func(t *gotest.T, x float64) {
	})
}

func (s *FooTestSuite) FuzzScale32(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, x float32) {
	})
}
`

// The splice is the one edit gotest makes to user source; each case here is
// a way it used to leave that source changed in meaning or uncompilable.
func (s *FuzzPromoteTestSuite) TestInsertFuzzAddKeepsTheSourceCompiling(t *gotest.T, _ *promoteCtx) {
	t.It("lands after a trailing comment instead of moving it onto the new seed", func(it *gotest.T) {
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimTrailingComment), "FooTestSuite", "FuzzTrim", []string{`"stale"`})
		gotest.NoError(it, err)
		gotest.True(it, found)
		gotest.Contains(it, string(edited), "\tf.Add(\"  x \") // note\n\tf.Add(\"stale\")\n")
	})

	t.It("adds the import a spliced expression needs", func(it *gotest.T) {
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzScale), "FooTestSuite", "FuzzScale", []string{`math.NaN()`})
		gotest.NoError(it, err)
		gotest.True(it, found)
		gotest.Contains(it, string(edited), `"math"`)
		gotest.Contains(it, string(edited), `f.Add(math.NaN())`)
	})

	t.When("the seed carries a corpus-file float special", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Desc   string
			method string
			expr   string
			want   string
		}{
			{"NaN", "FuzzScale", "float64(NaN)", "f.Add(math.NaN())"},
			{"+Inf", "FuzzScale", "float64(+Inf)", "f.Add(math.Inf(1))"},
			{"-Inf", "FuzzScale", "float64(-Inf)", "f.Add(math.Inf(-1))"},
			{"float32 NaN", "FuzzScale32", "float32(NaN)", "f.Add(float32(math.NaN()))"},
		}) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzScale), "FooTestSuite", tc.method, []string{tc.expr})
			gotest.NoError(sub, err)
			gotest.True(sub, found)
			gotest.Contains(sub, string(edited), tc.want)
			gotest.Contains(sub, string(edited), `"math"`)
		}
	})

	t.When("the seed's value count differs from the callback's arguments", func(w *gotest.T) {
		w.It("refuses and names both counts", func(it *gotest.T) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimOneSeed), "FooTestSuite", "FuzzTrim", []string{`"a"`, `"b"`, `[]byte("c")`})
			gotest.True(it, found)
			gotest.ErrorContains(it, err, "takes 1 argument")
			gotest.ErrorContains(it, err, "3 values")
			gotest.Nil(it, edited)
		})
	})
}

func (s *FuzzPromoteTestSuite) TestPromoteFuzzSeed(t *gotest.T, ctx *promoteCtx) {
	t.It("writes the seed to disk and returns the file and line", func(it *gotest.T) {
		path := s.writeSuite(it, ctx, fuzzTrimOneSeed)
		gotPath, line, err := refactor.PromoteFuzzSeed(ctx.dir, "FooTestSuite", "FuzzTrim", []string{`"stale"`})
		gotest.NoError(it, err)
		gotest.Equal(it, path, gotPath)
		gotest.Greater(it, line, 0)
		gotest.Contains(it, s.readSuite(it, path), `f.Add("stale")`)
	})

	t.When("the method has no *gotest.F parameter", func(w *gotest.T) {
		w.It("propagates the error and leaves the file untouched", func(it *gotest.T) {
			path := s.writeSuite(it, ctx, fuzzTrimNoFParam)
			_, _, err := refactor.PromoteFuzzSeed(ctx.dir, "FooTestSuite", "FuzzTrim", []string{`"stale"`})
			gotest.Error(it, err)
			gotest.Equal(it, fuzzTrimNoFParam, s.readSuite(it, path))
		})
	})

	t.When("the method does not exist", func(w *gotest.T) {
		w.It("errors and leaves the file untouched", func(it *gotest.T) {
			path := s.writeSuite(it, ctx, fuzzTrimNoSeed)
			_, _, err := refactor.PromoteFuzzSeed(ctx.dir, "FooTestSuite", "FuzzMissing", []string{`"stale"`})
			gotest.Error(it, err)
			gotest.Equal(it, fuzzTrimNoSeed, s.readSuite(it, path))
		})
	})
}
