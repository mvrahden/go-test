package gotestrunner_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ArgsTestSuite covers the go test flag vocabulary the runner speaks: which flags are
// build-only or run-only, how -run, -parallel, -bench and -coverprofile are
// read, rewritten and forwarded, and how the setup timeout resolves.
type ArgsTestSuite struct{}

func (s *ArgsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ArgsTestSuite) TestIsGoTestFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name    string
		flag    string
		isValue bool
		known   bool
	}{
		{"build bool", "-race", false, true},
		{"build value", "-tags", true, true},
		{"build special value", "-o", true, true},
		{"run bool", "-v", false, true},
		{"run value", "-run", true, true},
		{"json", "-json", false, true},
		{"args", "-args", false, true},
		{"unknown", "-zzz", false, false},
		{"double dash unknown", "--debug", false, false},
	}) {
		isValue, known := gotestrunner.IsGoTestFlag(tc.flag)
		gotest.Equal(sub, tc.isValue, isValue)
		gotest.Equal(sub, tc.known, known)
	}
}

func (s *ArgsTestSuite) TestCoverProfile(t *gotest.T) {
	t.When("extracting from flags", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name   string
			flags  []string
			expect string
		}{
			{"empty", nil, ""},
			{"equals form", []string{"-v", "-coverprofile=cover.out"}, "cover.out"},
			{"space form", []string{"-coverprofile", "cover.out", "-v"}, "cover.out"},
			{"stops at -args", []string{"-args", "-coverprofile=cover.out"}, ""},
			{"no coverprofile", []string{"-v", "-count=1"}, ""},
		}) {
			got := gotestrunner.ExtractCoverProfile(tc.flags)
			gotest.Equal(sub, tc.expect, got)
		}
	})

	t.When("stripping from flags", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name   string
			flags  []string
			expect []string
		}{
			{"empty", nil, nil},
			{"equals form", []string{"-v", "-coverprofile=cover.out", "-count=1"}, []string{"-v", "-count=1"}},
			{"space form", []string{"-coverprofile", "cover.out", "-v"}, []string{"-v"}},
			{"preserves -args passthrough", []string{"-v", "-args", "-coverprofile=x"}, []string{"-v", "-args", "-coverprofile=x"}},
			{"no coverprofile unchanged", []string{"-v", "-count=1"}, []string{"-v", "-count=1"}},
		}) {
			got := gotestrunner.StripCoverProfile(tc.flags)
			gotest.Equal(sub, tc.expect, got)
		}
	})

	t.When("merging profiles", func(w *gotest.T) {
		w.When("two profiles with overlapping blocks", func(w2 *gotest.T) {
			w2.It("merges and sorts with max-count aggregation", func(it *gotest.T) {
				dir := it.TempDir()

				writeProfile := func(name, content string) string {
					p := filepath.Join(dir, name)
					err := os.WriteFile(p, []byte(content), 0o600)
					gotest.NoError(it, err)
					return p
				}

				p1 := writeProfile("a.out", "mode: set\nfoo/bar.go:1.2,3.4 1 1\nfoo/bar.go:5.6,7.8 1 0\n")
				p2 := writeProfile("b.out", "mode: set\nfoo/bar.go:5.6,7.8 1 1\nfoo/baz.go:1.2,3.4 1 1\n")

				out := filepath.Join(dir, "merged.out")
				err := gotestrunner.MergeCoverProfiles([]string{p1, p2}, out)
				gotest.NoError(it, err)

				data, err := os.ReadFile(out)
				gotest.NoError(it, err)
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")

				gotest.Equal(it, "mode: set", lines[0])
				gotest.Len(it, lines, 4)

				// Verify sorted order: foo/bar.go blocks before foo/baz.go
				gotest.True(it, strings.HasPrefix(lines[1], "foo/bar.go"))
				gotest.True(it, strings.HasPrefix(lines[2], "foo/bar.go"))
				gotest.True(it, strings.HasPrefix(lines[3], "foo/baz.go"))

				// Verify max-count aggregation: foo/bar.go:5.6,7.8 should be 1 (max of 0,1)
				gotest.True(it, strings.HasSuffix(lines[2], " 1"))
			})
		})

		w.When("profile A has uncovered block not in profile B", func(w2 *gotest.T) {
			w2.It("preserves uncovered blocks with count 0", func(it *gotest.T) {
				dir := it.TempDir()

				writeProfile := func(name, content string) string {
					p := filepath.Join(dir, name)
					err := os.WriteFile(p, []byte(content), 0o600)
					gotest.NoError(it, err)
					return p
				}

				pA := writeProfile("a.out", "mode: set\nfoo/bar.go:1.2,3.4 1 1\nfoo/bar.go:10.1,12.5 1 0\n")
				pB := writeProfile("b.out", "mode: set\nfoo/baz.go:1.2,3.4 1 1\n")

				out := filepath.Join(dir, "merged.out")
				err := gotestrunner.MergeCoverProfiles([]string{pA, pB}, out)
				gotest.NoError(it, err)

				data, err := os.ReadFile(out)
				gotest.NoError(it, err)
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")

				gotest.Len(it, lines, 4)
				gotest.Contains(it, lines, "foo/bar.go:10.1,12.5 1 0")
			})
		})

		w.When("one profile is missing", func(w2 *gotest.T) {
			w2.It("skips the missing file", func(it *gotest.T) {
				dir := it.TempDir()
				p := filepath.Join(dir, "exists.out")
				_ = os.WriteFile(p, []byte("mode: set\nfoo.go:1.2,3.4 1 1\n"), 0o600)

				out := filepath.Join(dir, "merged.out")
				err := gotestrunner.MergeCoverProfiles([]string{filepath.Join(dir, "missing.out"), p}, out)
				gotest.NoError(it, err)

				data := gotest.Must(os.ReadFile(out))
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				gotest.Len(it, lines, 2)
			})
		})
	})
}

func (s *ArgsTestSuite) TestSplitTopLevelOr(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		input  string
		expect []string
	}{
		{"no pipe", `^TestFoo$`, []string{`^TestFoo$`}},
		{"two alternatives", `^TestA$|^TestB$`, []string{`^TestA$`, `^TestB$`}},
		{"pipe inside parens", `^Test$/^(A|B)$`, []string{`^Test$/^(A|B)$`}},
		{"pipe inside brackets", `^Test[a|b]$`, []string{`^Test[a|b]$`}},
		{"mixed top and nested", `^TestA$/^(X|Y)$|^TestB$/^Z$`, []string{`^TestA$/^(X|Y)$`, `^TestB$/^Z$`}},
		{"escaped pipe", `^Test\|Foo$`, []string{`^Test\|Foo$`}},
		{"nested parens", `^Test$/^((A|B)|C)$`, []string{`^Test$/^((A|B)|C)$`}},
		{"three alternatives", `^A$|^B$|^C$`, []string{`^A$`, `^B$`, `^C$`}},
	}) {
		got := gotestrunner.ExportSplitTopLevelOr(tc.input)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestHasVerboseFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect bool
	}{
		{"empty flags", nil, false},
		{"no -v", []string{"-count=1", "-timeout=10m"}, false},
		{"-v present", []string{"-count=1", "-v"}, true},
		{"-v=true present", []string{"-v=true", "-timeout=10m"}, true},
		{"-v=false is not verbose", []string{"-v=false"}, false},
		{"-verbose is not -v", []string{"-verbose"}, false},
	}) {
		got := gotestrunner.HasVerboseFlag(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestSuiteRunFilter(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name         string
		userFilter   string
		testFuncName string
		expect       string
	}{
		{
			Name:         "empty filter",
			userFilter:   "",
			testFuncName: "TestFooSuite",
			expect:       "",
		},
		{
			Name:         "suite only (no subtest)",
			userFilter:   "^TestFooSuite$",
			testFuncName: "TestFooSuite",
			expect:       "",
		},
		{
			Name:         "single method filter",
			userFilter:   "^TestFooSuite$/^TestBar$",
			testFuncName: "TestFooSuite",
			expect:       "^TestFooSuite$/^TestBar$",
		},
		{
			Name:         "multi-method same suite",
			userFilter:   "^TestFooSuite$/^(TestBar|TestBaz)$",
			testFuncName: "TestFooSuite",
			expect:       "^TestFooSuite$/^(TestBar|TestBaz)$",
		},
		{
			Name:         "multi-suite picks matching",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteA",
			expect:       "^TestSuiteA$/^TestX$",
		},
		{
			Name:         "multi-suite picks other",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteB",
			expect:       "^TestSuiteB$/^TestY$",
		},
		{
			Name:         "multi-suite no match",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteC",
			expect:       "",
		},
	}) {
		got := gotestrunner.ExportSuiteRunFilter(tc.userFilter, tc.testFuncName)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestExtractParallelValue(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect int
	}{
		{"empty", nil, 0},
		{"not present", []string{"-v", "-timeout=10m"}, 0},
		{"equals form", []string{"-v", "-parallel=4"}, 4},
		{"space form", []string{"-parallel", "8", "-v"}, 8},
		{"stops at -args", []string{"-args", "-parallel=4"}, 0},
		{"invalid value", []string{"-parallel=abc"}, 0},
	}) {
		got := gotestrunner.ExtractParallelValue(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestInjectParallel(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		n      int
		expect []string
	}{
		{"injects into empty", nil, 4, []string{"-parallel=4"}},
		{"injects when absent", []string{"-v", "-timeout=10m"}, 2, []string{"-v", "-timeout=10m", "-parallel=2"}},
		{"skips when equals present", []string{"-parallel=8", "-v"}, 2, []string{"-parallel=8", "-v"}},
		{"skips when space present", []string{"-parallel", "8", "-v"}, 2, []string{"-parallel", "8", "-v"}},
		{"does not inject after -args", []string{"-v", "-args", "-parallel=9"}, 2, []string{"-v", "-args", "-parallel=9", "-parallel=2"}},
	}) {
		got := gotestrunner.InjectParallel(tc.flags, tc.n)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestExtractBenchFilter(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect string
	}{
		{"empty", nil, ""},
		{"not present", []string{"-v", "-timeout=10m"}, ""},
		{"equals form", []string{"-bench=Foo"}, "Foo"},
		{"space form", []string{"-bench", "Foo", "-v"}, "Foo"},
		{"stops at -args", []string{"-args", "-bench=Foo"}, ""},
	}) {
		got := gotestrunner.ExtractBenchFilter(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestStripBenchFilter(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect []string
	}{
		{"empty", nil, nil},
		{"equals form", []string{"-bench=Foo", "-v"}, []string{"-v"}},
		{"space form", []string{"-bench", "Foo", "-v"}, []string{"-v"}},
		{"leaves other flags untouched", []string{"-benchtime=2x", "-count=1"}, []string{"-benchtime=2x", "-count=1"}},
		{"does not strip past -args", []string{"-v", "-args", "-bench=Foo"}, []string{"-v", "-args", "-bench=Foo"}},
	}) {
		got := gotestrunner.StripBenchFilter(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestParseExecFlags(t *gotest.T) {
	t.When("parsing exec flags", func(w *gotest.T) {
		w.It("extracts verbose flag", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-v", "./..."})
			gotest.True(it, pf.Verbose)
		})
		w.It("extracts run filter", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-run", "TestFoo", "-v"})
			gotest.Equal(it, "TestFoo", pf.UserRunFilter)
		})
		w.It("extracts cover profile", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-coverprofile=cover.out", "-v"})
			gotest.Equal(it, "cover.out", pf.UserCoverProfile)
		})
		w.It("separates build and run flags", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-race", "-v", "-count=1"})
			gotest.Contains(it, pf.BuildFlags, "-race")
		})
		w.It("handles empty args", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags(nil)
			gotest.False(it, pf.Verbose)
			gotest.Empty(it, pf.UserRunFilter)
		})
	})
}

func (s *ArgsTestSuite) TestAssignCoverProfiles(t *gotest.T) {
	t.When("assigning cover profiles", func(w *gotest.T) {
		w.It("assigns sequential paths", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/a", SuiteName: "TestA"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/b", SuiteName: "TestB"}},
			}
			gotestrunner.ExportAssignCoverProfiles(targets, "/tmp/cover")
			gotest.Equal(it, filepath.Join("/tmp/cover", "0.out"), targets[0].CoverProfile)
			gotest.Equal(it, filepath.Join("/tmp/cover", "1.out"), targets[1].CoverProfile)
		})
	})
}

func (s *ArgsTestSuite) TestBuildExtraEnv(t *gotest.T) {
	t.When("building extra env", func(w *gotest.T) {
		w.It("includes snapshot flag when set", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{UpdateSnapshots: true}, nil)
			gotest.Equal(it, "1", env[protocol.EnvUpdateSnapshots])
		})
		w.It("omits snapshot flag when not set", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{}, nil)
			_, ok := env[protocol.EnvUpdateSnapshots]
			gotest.False(it, ok)
		})
		w.It("omits state file when no process", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{}, nil)
			_, ok := env[protocol.EnvSharedStateFile]
			gotest.False(it, ok)
		})
	})
}

func (s *ArgsTestSuite) TestResolveSetupTimeout(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		input  time.Duration
		expect time.Duration
	}{
		{"not set defaults to 2m", 0, 2 * time.Minute},
		{"positive passed through", 5 * time.Minute, 5 * time.Minute},
		{"negative means no limit", -1, 0},
		{"large negative means no limit", -30 * time.Second, 0},
		{"small positive passed through", 10 * time.Second, 10 * time.Second},
	}) {
		got := gotestrunner.ExportResolveSetupTimeout(tc.input)
		gotest.Equal(sub, tc.expect, got)
	}
}
