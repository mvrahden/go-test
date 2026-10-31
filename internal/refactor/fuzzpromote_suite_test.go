package refactor_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

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
	cfg.Timeout = 90 * time.Second // go/packages loads per case run long under CI load
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
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimOneSeed), "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
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
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimNoSeed), "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
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
		edited, found, err := refactor.InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzPair", []string{`"stale"`, `int64(-3)`}, nil)
		gotest.NoError(it, err)
		gotest.True(it, found)
		gotest.Contains(it, string(edited), `f.Add("stale", int64(-3))`)
	})

	t.It("lands after every existing f.Add, not only the first", func(it *gotest.T) {
		src := strings.Replace(fuzzTrimOneSeed, "\tf.Add(\"  x \")\n", "\tf.Add(\"  x \")\n\tf.Add(\"y\")\n", 1)
		edited, found, err := refactor.InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
		gotest.NoError(it, err)
		gotest.True(it, found)
		out := string(edited)
		gotest.Greater(it, strings.Index(out, `f.Add("stale")`), strings.Index(out, `f.Add("y")`), "output:\n%s", out)
	})

	t.When("the suite is not in the file", func(w *gotest.T) {
		w.It("reports not found without an edit", func(it *gotest.T) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimNoSeed), "OtherTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
			gotest.NoError(it, err)
			gotest.False(it, found)
			gotest.Nil(it, edited)
		})
	})

	t.When("the method has no *gotest.F parameter", func(w *gotest.T) {
		w.It("is found but refuses to guess", func(it *gotest.T) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimNoFParam), "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
			gotest.True(it, found)
			gotest.Error(it, err)
			gotest.Nil(it, edited)
		})
	})

	t.When("the method has no body", func(w *gotest.T) {
		w.It("is found but refuses to guess", func(it *gotest.T) {
			// Legal Go for an externally implemented method; nowhere to splice.
			src := "package example\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FooTestSuite struct{}\n\nfunc (s *FooTestSuite) FuzzTrim(f *gotest.F)\n"
			edited, found, err := refactor.InsertFuzzAdd([]byte(src), "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
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
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimTrailingComment), "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
		gotest.NoError(it, err)
		gotest.True(it, found)
		gotest.Contains(it, string(edited), "\tf.Add(\"  x \") // note\n\tf.Add(\"stale\")\n")
	})

	t.It("adds the import a spliced expression needs", func(it *gotest.T) {
		edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzScale), "FooTestSuite", "FuzzScale", []string{`math.NaN()`}, nil)
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
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzScale), "FooTestSuite", tc.method, []string{tc.expr}, nil)
			gotest.NoError(sub, err)
			gotest.True(sub, found)
			gotest.Contains(sub, string(edited), tc.want)
			gotest.Contains(sub, string(edited), `"math"`)
		}
	})

	t.When("the seed's value count differs from the callback's arguments", func(w *gotest.T) {
		w.It("refuses and names both counts", func(it *gotest.T) {
			edited, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimOneSeed), "FooTestSuite", "FuzzTrim", []string{`"a"`, `"b"`, `[]byte("c")`}, nil)
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
		gotPath, line, err := refactor.PromoteFuzzSeed(ctx.dir, "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
		gotest.NoError(it, err)
		gotest.Equal(it, path, gotPath)
		gotest.Greater(it, line, 0)
		gotest.Contains(it, s.readSuite(it, path), `f.Add("stale")`)
	})

	t.When("the method has no *gotest.F parameter", func(w *gotest.T) {
		w.It("propagates the error and leaves the file untouched", func(it *gotest.T) {
			path := s.writeSuite(it, ctx, fuzzTrimNoFParam)
			_, _, err := refactor.PromoteFuzzSeed(ctx.dir, "FooTestSuite", "FuzzTrim", []string{`"stale"`}, nil)
			gotest.Error(it, err)
			gotest.Equal(it, fuzzTrimNoFParam, s.readSuite(it, path))
		})
	})

	t.When("the method does not exist", func(w *gotest.T) {
		w.It("errors and leaves the file untouched", func(it *gotest.T) {
			path := s.writeSuite(it, ctx, fuzzTrimNoSeed)
			_, _, err := refactor.PromoteFuzzSeed(ctx.dir, "FooTestSuite", "FuzzMissing", []string{`"stale"`}, nil)
			gotest.Error(it, err)
			gotest.Equal(it, fuzzTrimNoSeed, s.readSuite(it, path))
		})
	})
}

// seedTypes knows the types the literals below convert to.
func seedTypes(qual, name string) bool {
	switch qual + "." + name {
	case ".Priority", ".int64", ".float64", ".string", ".rune", "pkg.Level":
		return true
	}
	return false
}

// A seed is spliced only when every argument is a literal value: a call, a
// function literal or an operator is code, never a seed.
func (s *FuzzPromoteTestSuite) TestInsertFuzzAddRefusesCode(t *gotest.T, _ *promoteCtx) {
	for sub, c := range gotest.Each(t, []struct {
		Desc string
		expr string
	}{
		{Desc: "a call", expr: `os.Getenv("HOME")`},
		{Desc: "a call of a local function", expr: `evil(1)`},
		{Desc: "a builtin call", expr: `string(rune(len("ab")))`},
		{Desc: "a function literal", expr: `func() string { return "x" }()`},
		{Desc: "an operator", expr: `"a" + "b"`},
		{Desc: "an identifier", expr: "home"},
		{Desc: "a call inside a literal", expr: `Message{To: os.Getenv("HOME")}`},
		{Desc: "a receive", expr: "<-ch"},
		{Desc: "two expressions", expr: `"a"), os.Exit(1`},
	}) {
		_, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimOneSeed), "FooTestSuite", "FuzzTrim", []string{c.expr}, seedTypes)
		gotest.True(sub, found)
		gotest.ErrorContains(sub, err, "not a literal")
	}
}

// The literals a corpus file or a codec's echo spell are spliced as written.
func (s *FuzzPromoteTestSuite) TestInsertFuzzAddTakesLiterals(t *gotest.T, _ *promoteCtx) {
	for sub, c := range gotest.Each(t, []struct {
		Desc string
		expr string
	}{
		{Desc: "a string", expr: `"a@\x00"`},
		{Desc: "a conversion", expr: "int64(-3)"},
		{Desc: "a byte slice", expr: `[]byte("\xff")`},
		{Desc: "a float special", expr: "float64(+Inf)"},
		{Desc: "float bits", expr: "math.Float64frombits(0x7ff8000000000001)"},
		{Desc: "a struct literal", expr: `Message{To: "a", Count: Priority(2), Tags: []string{"x"}, At: &Point{X: -1}, N: &[]int{3}[0], F: math.NaN(), G: math.Inf(-1), P: nil, Ok: true}`},
		{Desc: "a qualified type", expr: `pkg.Message{To: "a", L: pkg.Level(3)}`},
	}) {
		_, found, err := refactor.InsertFuzzAdd([]byte(fuzzTrimOneSeed), "FooTestSuite", "FuzzTrim", []string{c.expr}, seedTypes)
		gotest.True(sub, found)
		gotest.NoError(sub, err)
	}
}
