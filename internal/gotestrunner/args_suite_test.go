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
		{"skip value", "-skip", true, true},
		{"json", "-json", false, true},
		{"args", "-args", false, true},
		{"unknown", "-zzz", false, false},
		{"double dash unknown", "--debug", false, false},
		{"vet is a build value flag", "-vet", true, true},
	}) {
		isValue, known := gotestrunner.IsGoTestFlag(tc.flag)
		gotest.Equal(sub, tc.isValue, isValue)
		gotest.Equal(sub, tc.known, known)
	}
}

// Every positional argument that is not a flag's value is a package
// pattern, as go test reads it, whatever its shape.
func (s *ArgsTestSuite) TestValidateTestFlags(t *gotest.T) {
	t.When("every value is one the test binary takes", func(w *gotest.T) {
		w.It("passes them", func(it *gotest.T) {
			gotest.NoError(it, gotestrunner.ValidateTestFlags([]string{
				"-count=2", "-parallel", "4", "-timeout=5m", "-v", "-shuffle=on", "-shuffle=7",
				"-cpu=1,2", "-run", "TestA/(x|y)", "-skip=TestB", "-list", "[]|x]", "-race", "./...", "-args", "-count=nope",
			}))
		})
	})
	t.When("a flag is repeated", func(w *gotest.T) {
		w.It("checks only the last value, the one the test binary keeps", func(it *gotest.T) {
			gotest.NoError(it, gotestrunner.ValidateTestFlags([]string{"-run=[", "-run=TestB", "-skip", "(", "-skip", "TestA", "-shuffle=xyz", "-shuffle=on"}))
			gotest.ErrorContains(it, gotestrunner.ValidateTestFlags([]string{"-run=TestB", "-run=["}), "invalid regexp for element 0 of -run")
		})
	})
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
		want string
	}{
		{"a count that is no number", []string{"-count=abc"}, `invalid value "abc" for flag -count`},
		{"a parallel given apart from its flag", []string{"-parallel", "abc"}, `invalid value "abc" for flag -parallel`},
		{"a timeout that is no duration", []string{"-timeout=bogus"}, `invalid value "bogus" for flag -timeout`},
		{"a shuffle that is neither on, off nor a seed", []string{"-shuffle=xyz"}, `invalid value "xyz" for flag -shuffle`},
		{"a cpu list with a word", []string{"-cpu=1,x"}, `invalid value "1,x" for flag -cpu`},
		{"a run pattern that is no regexp", []string{"-run=["}, `invalid regexp for element 0 of -run ("["): error parsing regexp`},
		{"a skip pattern that is no regexp", []string{"-skip", "("}, `invalid regexp for element 0 of -skip ("("): error parsing regexp`},
		{"a run pattern whose subtest level is no regexp", []string{"-run=TestA/["}, `invalid regexp for element 1 of -run ("[")`},
		{"a skip alternative cut at a top-level bar", []string{"-skip=[]|Nothing]"}, `invalid regexp for alternation 0 of element 0 of -skip ("[]")`},
		{"a skip alternative cut inside a quoted span", []string{`-skip=\QNothing|x\E`}, `invalid regexp for alternation 1 of element 0 of -skip ("x\\E")`},
		{"a bench alternative that is no regexp", []string{"-bench", "[]|x]"}, `invalid regexp for alternation 0 of element 0 of -bench ("[]")`},
		{"a list pattern that is no regexp", []string{"-list=("}, `invalid regexp in -list ("("): error parsing regexp`},
	}) {
		gotest.ErrorContains(t, gotestrunner.ValidateTestFlags(tc.args), tc.want)
	}
}

func (s *ArgsTestSuite) TestPackagePatterns(t *gotest.T) {
	t.It("classifies meta patterns and bare import paths as patterns", func(it *gotest.T) {
		c := gotestrunner.ClassifyGoTestArgs([]string{"-run", "X", "std", "fmt", "-count", "2", "./a"})
		gotest.Equal(it, []string{"std", "fmt", "./a"}, c.PkgPatterns)
		gotest.Equal(it, []string{"-run", "X", "-count", "2"}, c.RunFlags)
	})
	t.It("gives -skip the non-flag after it, as -run", func(it *gotest.T) {
		c := gotestrunner.ClassifyGoTestArgs([]string{"-skip", "X", "fmt"})
		gotest.Equal(it, []string{"fmt"}, c.PkgPatterns)
		gotest.Equal(it, []string{"-skip", "X"}, c.RunFlags)
	})
	t.It("hands an unknown flag and the non-flag after it to the test binary", func(it *gotest.T) {
		c := gotestrunner.ClassifyGoTestArgs([]string{"-custom", "value", "fmt"})
		gotest.Empty(it, c.BuildFlags)
		gotest.Equal(it, []string{"-args", "-custom", "value"}, c.RunFlags)
		gotest.Equal(it, []string{"fmt"}, c.PkgPatterns)
	})
	t.It("places unknown flags after the run flags it knows, as go test does", func(it *gotest.T) {
		c := gotestrunner.ClassifyGoTestArgs([]string{"-custom=1", "-v", "fmt", "-count=2"})
		gotest.Equal(it, []string{"-v", "-count=2", "-args", "-custom=1"}, c.RunFlags)
		gotest.Equal(it, []string{"fmt"}, c.PkgPatterns)
	})
	t.It("keeps what follows -args verbatim behind the unknown flags", func(it *gotest.T) {
		c := gotestrunner.ClassifyGoTestArgs([]string{"-custom=1", "fmt", "-args", "-count=nope"})
		gotest.Equal(it, []string{"-args", "-custom=1", "-count=nope"}, c.RunFlags)
	})
}

func (s *ArgsTestSuite) TestTranslateToTestBinaryFlags(t *gotest.T) {
	t.When("run flags are turned into the binary's flags", func(w *gotest.T) {
		w.It("prefixes the flags the binary's testing package owns", func(it *gotest.T) {
			gotest.Equal(it, []string{"-test.v", "-test.count=2"}, gotestrunner.TranslateToTestBinaryFlags([]string{"-v", "-count=2"}))
		})
		w.It("consumes the -args word and passes what follows verbatim", func(it *gotest.T) {
			gotest.Equal(it, []string{"-test.v", "-custom=1", "-count=nope"}, gotestrunner.TranslateToTestBinaryFlags([]string{"-v", "-args", "-custom=1", "-count=nope"}))
		})
	})
}

// -vet reaches `go test -c`, where vet runs, and must never reach
// packages.Load, which rejects it like the cover flags.
func (s *ArgsTestSuite) TestVetFlag(t *gotest.T) {
	t.It("is classified as a build flag in both spellings", func(it *gotest.T) {
		c := gotestrunner.ClassifyGoTestArgs([]string{"-vet=off", "-v"})
		gotest.Equal(it, []string{"-vet=off"}, c.BuildFlags)
		gotest.Equal(it, []string{"-v"}, c.RunFlags)

		c = gotestrunner.ClassifyGoTestArgs([]string{"-vet", "off", "-v"})
		gotest.Equal(it, []string{"-vet", "off"}, c.BuildFlags)
		gotest.Equal(it, []string{"-v"}, c.RunFlags)
	})

	t.It("is stripped before the load, with the cover flags", func(it *gotest.T) {
		gotest.Equal(it, []string{"-race"}, gotestrunner.StripNonLoadFlags([]string{"-vet=off", "-race", "-cover"}))
		gotest.Equal(it, []string{"-race"}, gotestrunner.StripNonLoadFlags([]string{"-vet", "off", "-race", "-covermode", "atomic"}))
	})
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
		{"the last -v wins over an earlier one", []string{"-v", "-v=false"}, false},
		{"a later -v wins over -v=false", []string{"-v=false", "-v"}, true},
		{"-v after -args is the binary's", []string{"-args", "-v"}, false},
		{"-test.v is -v", []string{"-test.v=true"}, true},
		{"-v=test2json is verbose", []string{"-v=test2json"}, true},
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

func (s *ArgsTestSuite) TestExtractCountValue(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name  string
		flags []string
		want  int
	}{
		{"absent is one run", []string{"-benchtime=1x"}, 1},
		{"joined", []string{"-count=6"}, 6},
		{"separate", []string{"-count", "4", "-v"}, 4},
		{"test prefix", []string{"-test.count=3"}, 3},
		{"behind -args is the binary's", []string{"-args", "-count=9"}, 1},
		{"the last occurrence wins", []string{"-count=2", "-count", "5"}, 5},
		{"unparseable is one run", []string{"-count=many"}, 1},
	}) {
		gotest.Equal(sub, tc.want, gotestrunner.ExtractCountValue(tc.flags))
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
		{"the last occurrence wins", []string{"-parallel=4", "-parallel", "2"}, 2},
		{"invalid value", []string{"-parallel=abc"}, 0},
	}) {
		got := gotestrunner.ExtractParallelValue(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ArgsTestSuite) TestInjectDefaultTimeout(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect []string
	}{
		{"adds the default", []string{"-v"}, []string{"-v", "-timeout=10m"}},
		{"keeps a user timeout", []string{"-timeout=1m", "-v"}, []string{"-timeout=1m", "-v"}},
		{"injects before -args, never behind it", []string{"-v", "-args", "-timeout=9s"}, []string{"-v", "-timeout=10m", "-args", "-timeout=9s"}},
	}) {
		gotest.Equal(sub, tc.expect, gotestrunner.InjectDefaultTimeout(tc.flags))
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
		{"injects before -args, never behind it", []string{"-v", "-args", "-parallel=9"}, 2, []string{"-v", "-parallel=2", "-args", "-parallel=9"}},
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
		{"the last occurrence wins", []string{"-bench=Foo", "-bench", "Bar"}, "Bar"},
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
		w.It("takes the last of a repeated -run, -skip and -coverprofile, as the flag package does", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-run=TestA", "-skip", "TestB", "-coverprofile=a.out", "-run", "TestC", "-skip=TestD", "-coverprofile", "b.out"})
			gotest.Equal(it, "TestC", pf.UserRunFilter)
			gotest.Equal(it, "TestD", pf.UserSkipFilter)
			gotest.Equal(it, "b.out", pf.UserCoverProfile)
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

func (s *ArgsTestSuite) TestBuildBaseEnv(t *gotest.T) {
	t.When("building the base env", func(w *gotest.T) {
		w.It("includes snapshot flag when set", func(it *gotest.T) {
			env := gotestrunner.ExportBuildBaseEnv(gotestrunner.PipelineConfig{UpdateSnapshots: true})
			gotest.Contains(it, env, protocol.EnvUpdateSnapshots+"=1")
		})
		w.It("omits snapshot flag when not set", func(it *gotest.T) {
			env := gotestrunner.ExportBuildBaseEnv(gotestrunner.PipelineConfig{})
			gotest.NotContains(it, env, protocol.EnvUpdateSnapshots+"=1")
		})
	})
}

func (s *ArgsTestSuite) TestResolveSetupTimeout(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		input  time.Duration
		expect time.Duration
	}{
		{"not set is the fixtures' declared budgets", 0, 0},
		{"positive passed through", 5 * time.Minute, 5 * time.Minute},
		{"negative means no limit", -1, -1},
		{"large negative means no limit", -30 * time.Second, -1},
		{"small positive passed through", 10 * time.Second, 10 * time.Second},
	}) {
		got := gotestrunner.ExportResolveSetupTimeout(tc.input)
		gotest.Equal(sub, tc.expect, got)
	}
}
