package gotestgen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FirstContactTestSuite covers what a developer reads when a suite is
// declared wrongly: a generation error names the place it is about, and what
// generation passes over in silence is collected as a warning.
type FirstContactTestSuite struct{}

func (s *FirstContactTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func firstContact(t *gotest.T, pkg string) []*gotestgen.LoadResult {
	cwd, err := os.Getwd()
	gotest.NoError(t, err)
	loaded, broken, err := gotestgen.LoadPackages([]string{filepath.Join(cwd, "testdata", "firstcontact", pkg)}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)
	gotest.Len(t, loaded, 1)
	return loaded
}

func (s *FirstContactTestSuite) TestGenerationErrorsNameTheirPosition(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		pkg  string
		want []string
	}{
		{
			Desc: "every unsupported hook signature, at the hook",
			pkg:  "badhook",
			want: []string{
				`testdata/firstcontact/badhook/suite_test.go:7:25: unsupported signature for "HookTestSuite.BeforeEach": expected exactly 1 parameter`,
				`testdata/firstcontact/badhook/suite_test.go:9:25: unsupported signature for "HookTestSuite.AfterEach": expected 1 or 2 parameters and no return values`,
			},
		},
		{
			Desc: "a missing context parameter, at the method that lacks it",
			pkg:  "missingctx",
			want: []string{`testdata/firstcontact/missingctx/suite_test.go:13:24: CtxTestSuite.TestWithoutContext: suite has a returning BeforeEach`},
		},
		{
			Desc: "an argument that cannot be fuzzed, at the f.Fuzz call",
			pkg:  "fuzzreject",
			want: []string{`testdata/firstcontact/fuzzreject/suite_test.go:14:2: fuzz target FuzzFrameTestSuite_FuzzFrame: Frame.Done (chan struct{}) is not fuzzable`},
		},
		{
			Desc: "a TestMain that runs the tests itself in a fixture package, at the TestMain",
			pkg:  "testmainrun",
			want: []string{`testdata/firstcontact/testmainrun/main_test.go:8:1: TestMain must call gotestruntime.Main(m) so fixtures tear down after the tests: replace m.Run() with gotestruntime.Main(m)`},
		},
		{
			Desc: "a fixture the suite cannot bind, at the suite",
			pkg:  "badfixture",
			want: []string{`testdata/firstcontact/badfixture/suite_test.go:15:6: suite StoreTestSuite has benchmark methods but fixture StoreFixture defines BeforeEach/AfterEach`},
		},
	}) {
		_, _, err := gotestgen.GenerateFromLoaded(firstContact(t, tc.pkg))
		gotest.Error(t, err)
		lines := strings.Split(filepath.ToSlash(err.Error()), "\n")
		gotest.Len(t, lines, len(tc.want))
		for i, want := range tc.want {
			gotest.Regexp(t, "^"+regexp.QuoteMeta(want), lines[i])
		}
	}
}

func (s *FirstContactTestSuite) TestWarnings(t *gotest.T) {
	loaded := firstContact(t, "typos")
	result := gotestgen.NewCollector().CollectSuiteSpecs(loaded[0].Ptest)
	gotest.Empty(t, result.Errs)

	type warning struct {
		Line int
		Rule string
		Msg  string
	}
	var got []warning
	for _, w := range result.Warnings {
		got = append(got, warning{loaded[0].Ptest.Fset.Position(w.Pos).Line, w.Rule, w.Msg})
	}

	t.It("names every method that reads like harness and never runs", func(it *gotest.T) {
		gotest.Equal(it, []warning{
			{7, "lifecycle-typo", "method BeforAll on suite TypoTestSuite is similar to lifecycle hook BeforeAll"},
			{9, "x-lifecycle", "X_ prefix on lifecycle hook TypoTestSuite.X_AfterAll has no effect — remove the prefix or the method"},
			{12, "lifecycle-typo", "method AfterCall on suite TypoTestSuite is similar to lifecycle hook AfterAll"},
			{18, "", "Helpers.BenchmarkLookup takes *gotest.B, but Helpers is not a test suite (its name must end in TestSuite): the benchmark never runs"},
			{20, "", "Helpers.FuzzParse takes *gotest.F, but Helpers is not a test suite (its name must end in TestSuite): the fuzz target never runs"},
		}, got)
	})
	t.It("keeps the suite", func(it *gotest.T) {
		gotest.Len(it, result.Suites, 1)
	})
}

func (s *FirstContactTestSuite) TestFuzzRejections(t *gotest.T) {
	loaded := firstContact(t, "fuzzreject")
	result := gotestgen.NewCollector().CollectSuiteSpecs(loaded[0].Ptest)
	gotest.Empty(t, result.Errs)

	rejections := gotestgen.FuzzRejections(loaded[0].Ptest, result.Suites)
	gotest.Len(t, rejections, 1)
	gotest.ErrorContains(t, rejections[0].Err, "Frame.Done (chan struct{}) is not fuzzable")
	gotest.Equal(t, 14, loaded[0].Ptest.Fset.Position(rejections[0].Pos).Line)
}
