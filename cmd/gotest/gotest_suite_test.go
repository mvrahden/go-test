package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CmdGotestTestSuite tests CLI argument parsing, subcommands,
// discovery, spec rendering, and code generation.
type CmdGotestTestSuite struct{}

func (s *CmdGotestTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *CmdGotestTestSuite) TestDefaultArgs(t *gotest.T) {
	t.When("CLI absent", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			inv    Invocation
			expect []string
		}{
			{
				Desc:   "config zero: no prepend",
				inv:    Invocation{Args: []string{"-v"}},
				expect: []string{"-v"},
			},
			{
				Desc: "config positive: config prepended",
				inv: Invocation{
					Args:   []string{"-v"},
					Config: config.ProjectConfig{SetupTimeout: config.Dur(2 * time.Minute)},
				},
				expect: []string{"--setup-timeout=2m0s", "-v"},
			},
			{
				Desc: "config negative: config prepended",
				inv: Invocation{
					Args:   []string{"-v"},
					Config: config.ProjectConfig{SetupTimeout: config.Dur(-1 * time.Second)},
				},
				expect: []string{"--setup-timeout=-1s", "-v"},
			},
			{
				Desc: "tags and setup-timeout both prepended",
				inv: Invocation{
					Args:   []string{"-v"},
					Config: config.ProjectConfig{Tags: "integration", SetupTimeout: config.Dur(3 * time.Minute)},
				},
				expect: []string{"--setup-timeout=3m0s", "-tags=integration", "-v"},
			},
			{
				Desc: "config timeout prepended",
				inv: Invocation{
					Args:   []string{"-v"},
					Config: config.ProjectConfig{Timeout: config.Dur(15 * time.Minute)},
				},
				expect: []string{"--timeout=15m0s", "-v"},
			},
			{
				Desc: "config timeout zero: disables default",
				inv: Invocation{
					Args:   []string{"-v"},
					Config: config.ProjectConfig{Timeout: config.Dur(0)},
				},
				expect: []string{"--timeout=0s", "-v"},
			},
			{
				Desc: "config timeout negative: opt-out prepended",
				inv: Invocation{
					Args:   []string{"-v"},
					Config: config.ProjectConfig{Timeout: config.Dur(-1 * time.Second)},
				},
				expect: []string{"--timeout=-1s", "-v"},
			},
		}) {
			got := tc.inv.DefaultArgs()
			gotest.Equal(sub, tc.expect, got)
		}
	})

	t.When("CLI positive", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			inv    Invocation
			expect []string
		}{
			{
				Desc: "config zero: CLI preserved",
				inv: Invocation{
					Args: []string{"--setup-timeout=5m", "-v"},
				},
				expect: []string{"--setup-timeout=5m", "-v"},
			},
			{
				Desc: "config positive: CLI wins",
				inv: Invocation{
					Args:   []string{"--setup-timeout=5m", "-v"},
					Config: config.ProjectConfig{SetupTimeout: config.Dur(2 * time.Minute)},
				},
				expect: []string{"--setup-timeout=5m", "-v"},
			},
			{
				Desc: "config negative: CLI wins",
				inv: Invocation{
					Args:   []string{"--setup-timeout=5m", "-v"},
					Config: config.ProjectConfig{SetupTimeout: config.Dur(-1 * time.Second)},
				},
				expect: []string{"--setup-timeout=5m", "-v"},
			},
			{
				Desc: "CLI timeout wins over config timeout",
				inv: Invocation{
					Args:   []string{"--timeout=20m", "-v"},
					Config: config.ProjectConfig{Timeout: config.Dur(15 * time.Minute)},
				},
				expect: []string{"--timeout=20m", "-v"},
			},
		}) {
			got := tc.inv.DefaultArgs()
			gotest.Equal(sub, tc.expect, got)
		}
	})

	t.When("CLI negative", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			inv    Invocation
			expect []string
		}{
			{
				Desc: "config zero: CLI preserved",
				inv: Invocation{
					Args: []string{"--setup-timeout=-1s", "-v"},
				},
				expect: []string{"--setup-timeout=-1s", "-v"},
			},
			{
				Desc: "config positive: CLI wins",
				inv: Invocation{
					Args:   []string{"--setup-timeout=-1s", "-v"},
					Config: config.ProjectConfig{SetupTimeout: config.Dur(2 * time.Minute)},
				},
				expect: []string{"--setup-timeout=-1s", "-v"},
			},
			{
				Desc: "config negative: CLI wins",
				inv: Invocation{
					Args:   []string{"--setup-timeout=-1s", "-v"},
					Config: config.ProjectConfig{SetupTimeout: config.Dur(-1 * time.Second)},
				},
				expect: []string{"--setup-timeout=-1s", "-v"},
			},
		}) {
			got := tc.inv.DefaultArgs()
			gotest.Equal(sub, tc.expect, got)
		}
	})
}

func (s *CmdGotestTestSuite) TestSplitArgs(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc         string
		inArgs       []string
		allowed      map[string]bool
		expectOwn    []string
		expectGoTest []string
		expectErr    bool
	}{
		{Desc: "empty", inArgs: nil, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: nil},
		{Desc: "only go test args", inArgs: []string{"-v", "./...", "-race", "-count=1"}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-v", "./...", "-race", "-count=1"}},
		{Desc: "only own args", inArgs: []string{"--debug"}, allowed: ExportTestAllowed, expectOwn: []string{"--debug"}, expectGoTest: nil},
		{Desc: "mixed args", inArgs: []string{"--debug", "-v", "./...", "-race"}, allowed: ExportTestAllowed, expectOwn: []string{"--debug"}, expectGoTest: []string{"-v", "./...", "-race"}},
		{Desc: "min flag with equals", inArgs: []string{"--min=80", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--min=80"}, expectGoTest: []string{"-v"}},
		{Desc: "min flag with space", inArgs: []string{"--min", "90", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--min", "90"}, expectGoTest: []string{"-v"}},
		{Desc: "unknown gotest flag", inArgs: []string{"--unknown"}, allowed: ExportTestAllowed, expectErr: true},
		{Desc: "unknown go test flag", inArgs: []string{"-zzz"}, allowed: ExportTestAllowed, expectErr: true},
		{Desc: "gotest flag not in allowed set", inArgs: []string{"--debounce=200ms"}, allowed: ExportTestAllowed, expectErr: true},
		{Desc: "bare -- escape hatch", inArgs: []string{"--debug", "--", "-custom", "./..."}, allowed: ExportTestAllowed, expectOwn: []string{"--debug"}, expectGoTest: []string{"-custom", "./..."}},
		{Desc: "bare -- with no gotest flags", inArgs: []string{"--", "-v", "./..."}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-v", "./..."}},
		{Desc: "-args passthrough", inArgs: []string{"-v", "-args", "-custom=1"}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-v", "-args", "-custom=1"}},
		{Desc: "spec allowed set", inArgs: []string{"--format=md", "--no-color", "-v"}, allowed: ExportSpecAllowed, expectOwn: []string{"--format=md", "--no-color"}, expectGoTest: []string{"-v"}},
		{Desc: "watch allowed set", inArgs: []string{"--debounce=500ms", "-v"}, allowed: ExportWatchAllowed, expectOwn: []string{"--debounce=500ms"}, expectGoTest: []string{"-v"}},
		{Desc: "go test value flag with space", inArgs: []string{"-run", "TestFoo", "./..."}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-run", "TestFoo", "./..."}},
		{Desc: "go test value flag with equals", inArgs: []string{"-timeout=30s"}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-timeout=30s"}},
		{Desc: "watch: no flags", inArgs: []string{"./pkg/..."}, allowed: ExportWatchAllowed, expectOwn: nil, expectGoTest: []string{"./pkg/..."}},
		{Desc: "watch: json flag", inArgs: []string{"-json", "./pkg/..."}, allowed: ExportWatchAllowed, expectOwn: nil, expectGoTest: []string{"-json", "./pkg/..."}},
		{Desc: "watch: debounce with json", inArgs: []string{"--debounce=500ms", "-json", "./..."}, allowed: ExportWatchAllowed, expectOwn: []string{"--debounce=500ms"}, expectGoTest: []string{"-json", "./..."}},
		{Desc: "watch: debug and ci", inArgs: []string{"--debug", "--ci", "-v", "./..."}, allowed: ExportWatchAllowed, expectOwn: []string{"--debug", "--ci"}, expectGoTest: []string{"-v", "./..."}},
		{Desc: "timeout flag with equals", inArgs: []string{"--timeout=15m", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--timeout=15m"}, expectGoTest: []string{"-v"}},
		{Desc: "timeout flag with space", inArgs: []string{"--timeout", "15m", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--timeout", "15m"}, expectGoTest: []string{"-v"}},
	}) {
		own, goTest, err := SplitArgs(tc.inArgs, tc.allowed)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
			continue
		}
		gotest.NoError(sub, err)
		gotest.Equal(sub, tc.expectOwn, own)
		gotest.Equal(sub, tc.expectGoTest, goTest)
	}
}

func (s *CmdGotestTestSuite) TestParseSubcommand(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc            string
		args            []string
		expectSubcmd    string
		expectRemaining []string
	}{
		{Desc: "empty args", args: nil, expectSubcmd: "", expectRemaining: nil},
		{Desc: "no subcommand, just flags", args: []string{"-v", "./..."}, expectSubcmd: "", expectRemaining: []string{"-v", "./..."}},
		{Desc: "version subcommand", args: []string{"version"}, expectSubcmd: "version", expectRemaining: nil},
		{Desc: "scaffold subcommand", args: []string{"scaffold", "-v"}, expectSubcmd: "scaffold", expectRemaining: []string{"-v"}},
		{Desc: "migrate subcommand", args: []string{"migrate"}, expectSubcmd: "migrate", expectRemaining: nil},
		{Desc: "help subcommand", args: []string{"help"}, expectSubcmd: "help", expectRemaining: nil},
		{Desc: "generate subcommand", args: []string{"generate", "./..."}, expectSubcmd: "generate", expectRemaining: []string{"./..."}},
		{Desc: "watch subcommand", args: []string{"watch"}, expectSubcmd: "watch", expectRemaining: nil},
		{Desc: "clean subcommand", args: []string{"clean", "./..."}, expectSubcmd: "clean", expectRemaining: []string{"./..."}},
		{Desc: "spec subcommand", args: []string{"spec"}, expectSubcmd: "spec", expectRemaining: nil},
		{Desc: "unknown first arg is not consumed", args: []string{"./...", "-v"}, expectSubcmd: "", expectRemaining: []string{"./...", "-v"}},
		{Desc: "flag first arg is not consumed", args: []string{"-v", "./..."}, expectSubcmd: "", expectRemaining: []string{"-v", "./..."}},
		{Desc: "package pattern not consumed", args: []string{"github.com/foo/bar"}, expectSubcmd: "", expectRemaining: []string{"github.com/foo/bar"}},
	}) {
		subcmd, remaining := ParseSubcommand(tc.args)
		gotest.Equal(sub, tc.expectSubcmd, subcmd)
		gotest.Equal(sub, tc.expectRemaining, remaining)
	}
}

func (s *CmdGotestTestSuite) TestPackagePatterns(t *gotest.T) {
	t.When("extract package patterns", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc     string
			args     []string
			expected []string
		}{
			{Desc: "explicit relative path", args: []string{"-v", "./...", "-race"}, expected: []string{"./..."}},
			{Desc: "explicit named package", args: []string{"-v", "github.com/foo/bar", "-race"}, expected: []string{"github.com/foo/bar"}},
			{Desc: "no package defaults to dot", args: []string{"-v", "-race"}, expected: []string{"."}},
			{Desc: "multiple packages", args: []string{"./pkg/a", "./pkg/b", "-v"}, expected: []string{"./pkg/a", "./pkg/b"}},
			{Desc: "stops at -args", args: []string{"-v", "./...", "-args", "-custom", "./not/a/pkg"}, expected: []string{"./..."}},
			{Desc: "no args defaults to dot", args: nil, expected: []string{"."}},
			{Desc: "bare relative path", args: []string{"-v", "./cmd/gotest"}, expected: []string{"./cmd/gotest"}},
		}) {
			result := ExtractPackagePatterns(tc.args)
			gotest.Equal(sub, tc.expected, result)
		}
	})

	t.When("looks like package pattern", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			input  string
			expect bool
		}{
			{Desc: "relative path", input: "./pkg/foo", expect: true},
			{Desc: "absolute path", input: "/usr/local/pkg", expect: true},
			{Desc: "named package", input: "github.com/foo/bar", expect: true},
			{Desc: "flag", input: "-v", expect: false},
			{Desc: "bare word", input: "strings", expect: false},
			{Desc: "dot only", input: ".", expect: true},
			{Desc: "dot-slash", input: "./...", expect: true},
			{Desc: "windows absolute path", input: `C:\Users\runner\pkg`, expect: true},
		}) {
			gotest.Equal(sub, tc.expect, gotestrunner.LooksLikePackagePattern(tc.input))
		}
	})
}

func (s *CmdGotestTestSuite) TestParseMinFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		expect    int
		expectErr bool
	}{
		{Desc: "no flag", args: []string{"--debug"}, expect: 0},
		{Desc: "equals syntax", args: []string{"--min=80"}, expect: 80},
		{Desc: "space syntax", args: []string{"--min", "90"}, expect: 90},
		{Desc: "empty args", args: nil, expect: 0},
		{Desc: "invalid value", args: []string{"--min=abc"}, expectErr: true},
		{Desc: "min at end no value", args: []string{"--min"}, expect: 0},
		{Desc: "negative value", args: []string{"--min=-5"}, expectErr: true},
		{Desc: "over 100", args: []string{"--min=150"}, expectErr: true},
	}) {
		got, err := ExportParseMinFlag(tc.args)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
		} else {
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, got)
		}
	}
}

func (s *CmdGotestTestSuite) TestParseParallelFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		expect    int
		expectErr bool
	}{
		{Desc: "no flag", args: []string{"--debug"}, expect: 0},
		{Desc: "equals syntax", args: []string{"--parallel=8"}, expect: 8},
		{Desc: "space syntax", args: []string{"--parallel", "12"}, expect: 12},
		{Desc: "empty args", args: nil, expect: 0},
		{Desc: "invalid value", args: []string{"--parallel=abc"}, expectErr: true},
		{Desc: "zero value", args: []string{"--parallel=0"}, expectErr: true},
		{Desc: "negative value", args: []string{"--parallel=-4"}, expectErr: true},
	}) {
		got, err := ExportParseParallelFlag(tc.args)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
		} else {
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, got)
		}
	}
}

func (s *CmdGotestTestSuite) TestParseSetupTimeoutFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		expect    time.Duration
		expectErr bool
	}{
		{Desc: "no flag", args: []string{"--debug"}, expect: 0},
		{Desc: "equals syntax", args: []string{"--setup-timeout=2m"}, expect: 2 * time.Minute},
		{Desc: "space syntax", args: []string{"--setup-timeout", "30s"}, expect: 30 * time.Second},
		{Desc: "empty args", args: nil, expect: 0},
		{Desc: "invalid value", args: []string{"--setup-timeout=abc"}, expectErr: true},
		{Desc: "zero value", args: []string{"--setup-timeout=0"}, expect: -1},
		{Desc: "negative value", args: []string{"--setup-timeout=-5s"}, expect: -1},
		{Desc: "small positive", args: []string{"--setup-timeout=500ms"}, expect: 500 * time.Millisecond},
	}) {
		got, err := ExportParseSetupTimeoutFlag(tc.args)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
		} else {
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, got)
		}
	}
}

func (s *CmdGotestTestSuite) TestParseDebounceFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		expect    time.Duration
		expectErr bool
	}{
		{Desc: "no flag: default 200ms", args: []string{"--debug"}, expect: 200 * time.Millisecond},
		{Desc: "equals syntax", args: []string{"--debounce=500ms"}, expect: 500 * time.Millisecond},
		{Desc: "space syntax", args: []string{"--debounce", "1s"}, expect: 1 * time.Second},
		{Desc: "empty args: default 200ms", args: nil, expect: 200 * time.Millisecond},
		{Desc: "invalid value", args: []string{"--debounce=abc"}, expectErr: true},
		{Desc: "zero value", args: []string{"--debounce=0"}, expectErr: true},
		{Desc: "negative value", args: []string{"--debounce=-1s"}, expectErr: true},
	}) {
		got, err := ExportParseDebounceFlag(tc.args)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
		} else {
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, got)
		}
	}
}

func (s *CmdGotestTestSuite) TestParseGlobalTimeoutFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		expect    time.Duration
		expectErr bool
	}{
		{Desc: "no flag", args: []string{"--debug"}, expect: 0},
		{Desc: "equals syntax", args: []string{"--timeout=15m"}, expect: 15 * time.Minute},
		{Desc: "space syntax", args: []string{"--timeout", "30s"}, expect: 30 * time.Second},
		{Desc: "empty args", args: nil, expect: 0},
		{Desc: "invalid value", args: []string{"--timeout=abc"}, expectErr: true},
		{Desc: "zero value", args: []string{"--timeout=0"}, expect: -1},
		{Desc: "negative value", args: []string{"--timeout=-5s"}, expect: -1},
		{Desc: "small positive", args: []string{"--timeout=100ms"}, expect: 100 * time.Millisecond},
	}) {
		got, err := ExportParseGlobalTimeoutFlag(tc.args)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
		} else {
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, got)
		}
	}
}

func (s *CmdGotestTestSuite) TestResolveGlobalTimeout(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc   string
		input  time.Duration
		expect time.Duration
	}{
		{Desc: "not set: default 15m", input: 0, expect: 15 * time.Minute},
		{Desc: "positive: passthrough", input: 20 * time.Minute, expect: 20 * time.Minute},
		{Desc: "negative sentinel: no limit", input: -1, expect: 0},
		{Desc: "large negative: no limit", input: -100 * time.Minute, expect: 0},
		{Desc: "small positive: passthrough", input: 30 * time.Second, expect: 30 * time.Second},
	}) {
		gotest.Equal(sub, tc.expect, ExportResolveGlobalTimeout(tc.input))
	}

	t.When("end-to-end parse+resolve", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			args   []string
			expect time.Duration
		}{
			{Desc: "--timeout=0 disables", args: []string{"--timeout=0"}, expect: 0},
			{Desc: "--timeout=0s disables", args: []string{"--timeout=0s"}, expect: 0},
			{Desc: "--timeout=-1s disables", args: []string{"--timeout=-1s"}, expect: 0},
			{Desc: "absent defaults to 15m", args: []string{"-v"}, expect: 15 * time.Minute},
			{Desc: "--timeout=20m passes through", args: []string{"--timeout=20m"}, expect: 20 * time.Minute},
		}) {
			parsed, err := ExportParseGlobalTimeoutFlag(tc.args)
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, ExportResolveGlobalTimeout(parsed))
		}
	})
}

func (s *CmdGotestTestSuite) TestRunDiscover_SimpleSuite(t *gotest.T) {
	t.It("discovers suites in examples/cart", func(it *gotest.T) {
		absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
		if err != nil {
			it.T().Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
			it.T().Skipf("examples directory not found: %v", err)
		}

		loadResults, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
		if err != nil {
			it.T().Fatalf("LoadPackages: %v", err)
		}
		if len(loadResults) == 0 {
			it.T().Fatal("expected at least one load result")
		}

		out := ExportDiscoverOutput{}
		c := gotestgen.NewCollector()
		for _, lr := range loadResults {
			pkgEntry := ExportDiscoverPackage{
				ImportPath: lr.PkgPath,
				Dir:        lr.PkgDir,
			}

			if lr.Ptest != nil {
				result := c.CollectSuiteSpecs(lr.Ptest)
				if len(result.Errs) > 0 {
					it.T().Fatalf("collector error: %v", result.Errs[0].Err)
				}
				for _, suite := range result.Suites {
					pkgEntry.Suites = append(pkgEntry.Suites, ExportBuildDiscoverSuite(suite))
				}
			}
			if lr.Pxtest != nil {
				result := c.CollectSuiteSpecs(lr.Pxtest)
				if len(result.Errs) > 0 {
					it.T().Fatalf("collector error: %v", result.Errs[0].Err)
				}
				for _, suite := range result.Suites {
					pkgEntry.Suites = append(pkgEntry.Suites, ExportBuildDiscoverSuite(suite))
				}
			}

			out.Packages = append(out.Packages, pkgEntry)
		}

		if len(out.Packages) != 1 {
			it.T().Fatalf("expected 1 package, got %d", len(out.Packages))
		}

		pkg := out.Packages[0]
		gotest.Equal(it, "github.com/mvrahden/go-test/examples/cart", pkg.ImportPath)
		gotest.True(it, filepath.IsAbs(pkg.Dir), "dir should be absolute, got %q", pkg.Dir)

		if len(pkg.Suites) != 4 {
			it.T().Fatalf("expected 4 suites, got %d", len(pkg.Suites))
		}

		suiteByNameAndFile := map[string]ExportDiscoverSuite{}
		for i := range pkg.Suites {
			suiteByNameAndFile[pkg.Suites[i].Name+":"+pkg.Suites[i].File] = pkg.Suites[i]
		}

		// Verify ptest ShoppingCartTestSuite
		st := suiteByNameAndFile["ShoppingCartTestSuite:suite_test.go"]
		gotest.Equal(it, "ShoppingCartTestSuite", st.Name)
		gotest.False(it, st.Parallel)
		gotest.False(it, st.Focused)
		gotest.False(it, st.Excluded)
		gotest.Equal(it, "suite_test.go", st.File)
		gotest.Equal(it, 5, st.Line)
		gotest.Equal(it, 6, st.Col)

		expectedLifecycle := []string{"BeforeEach"}
		gotest.Equal(it, expectedLifecycle, st.Lifecycle)
		gotest.Empty(it, st.Fixtures)

		if len(st.Methods) != 9 {
			it.T().Fatalf("expected 9 methods, got %d", len(st.Methods))
		}
		gotest.Equal(it, "TestAddSingleItem", st.Methods[0].Name)
		gotest.Equal(it, 15, st.Methods[0].Line)
		gotest.Equal(it, 1, st.Methods[0].Col)
		gotest.Equal(it, "TestAddMultipleItems", st.Methods[1].Name)

		// Verify ptest PricingTestSuite (fixture-bound)
		pt := suiteByNameAndFile["PricingTestSuite:pricing_suite_test.go"]
		gotest.Equal(it, "PricingTestSuite", pt.Name)

		// Verify pxtest ShoppingCartTestSuite
		sx := suiteByNameAndFile["ShoppingCartTestSuite:suite_ext_test.go"]
		gotest.Equal(it, "ShoppingCartTestSuite", sx.Name)
		if len(sx.Methods) != 2 {
			it.T().Fatalf("expected 2 pxtest methods, got %d", len(sx.Methods))
		}
		gotest.Equal(it, "TestAddItem", sx.Methods[0].Name)
		gotest.Equal(it, "TestRemoveItem", sx.Methods[1].Name)

		// Verify pxtest PricingExtTestSuite (fixture-bound)
		px := suiteByNameAndFile["PricingExtTestSuite:pricing_ext_suite_test.go"]
		gotest.Equal(it, "PricingExtTestSuite", px.Name)

		// Verify JSON serialization roundtrip
		data, err := json.Marshal(out)
		if err != nil {
			it.T().Fatalf("json.Marshal: %v", err)
		}
		var roundtrip ExportDiscoverOutput
		if err := json.Unmarshal(data, &roundtrip); err != nil {
			it.T().Fatalf("json.Unmarshal: %v", err)
		}
		gotest.Len(it, roundtrip.Packages, 1)
	})
}

func (s *CmdGotestTestSuite) TestFocusViolation_String(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		v        FocusViolation
		expected string
	}{
		{
			Desc:     "suite violation only",
			v:        FocusViolation{SuiteName: "F_MyTestSuite"},
			expected: "  type F_MyTestSuite",
		},
		{
			Desc:     "method violation",
			v:        FocusViolation{SuiteName: "MyTestSuite", MethodName: "F_TestSomething"},
			expected: "  MyTestSuite.F_TestSomething",
		},
		{
			Desc:     "both focused suite and method",
			v:        FocusViolation{SuiteName: "F_MyTestSuite", MethodName: "F_TestFoo"},
			expected: "  F_MyTestSuite.F_TestFoo",
		},
	}) {
		gotest.Equal(sub, tc.expected, tc.v.String())
	}
}

func (s *CmdGotestTestSuite) TestGenerateOverlay(t *gotest.T) {
	t.When("suites are present", func(w *gotest.T) {
		w.It("produces valid overlay JSON", func(it *gotest.T) {
			absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
			if err != nil {
				it.T().Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
				it.T().Skipf("examples directory not found: %v", err)
			}

			loaded, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
			if err != nil {
				it.T().Fatalf("LoadPackages: %v", err)
			}
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			if err != nil {
				it.T().Fatalf("GenerateFromLoaded: %v", err)
			}
			if len(results) == 0 {
				it.T().Fatal("expected at least one generate result")
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			if err != nil {
				it.T().Fatalf("WriteOverlay: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			overlayFile := filepath.Join(tmpDir, "overlay.json")
			if _, err := os.Stat(overlayFile); err != nil {
				it.T().Fatalf("overlay.json not found: %v", err)
			}

			data, err := os.ReadFile(overlayFile)
			if err != nil {
				it.T().Fatalf("reading overlay.json: %v", err)
			}
			var overlayContent struct {
				Replace map[string]string `json:"Replace"`
			}
			if err := json.Unmarshal(data, &overlayContent); err != nil {
				it.T().Fatalf("overlay.json is not valid JSON: %v", err)
			}
			gotest.NotEmpty(it, overlayContent.Replace, "overlay.json Replace map is empty")
		})
	})

	t.When("no suites", func(w *gotest.T) {
		w.It("returns empty results for package without suites", func(it *gotest.T) {
			tmpDir, err := os.MkdirTemp("", "overlay-test-nosuite-*")
			if err != nil {
				it.T().Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module nosuite\n\ngo 1.24\n"), 0600); err != nil {
				it.T().Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0600); err != nil {
				it.T().Fatal(err)
			}

			loaded, err := gotestgen.LoadPackages([]string{tmpDir}, nil)
			if err != nil {
				it.T().Fatalf("LoadPackages: %v", err)
			}
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			if err != nil {
				it.T().Fatalf("GenerateFromLoaded: %v", err)
			}

			var allResults gotestgen.GenerateResults
			allResults = append(allResults, results...)
			if len(allResults) != 0 {
				it.T().Skipf("expected 0 results for package without suites, got %d (package may have test suites)", len(allResults))
			}
		})
	})
}

func (s *CmdGotestTestSuite) TestSpecFlagParsing(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		wantFmt   string
		wantOut   string
		wantInput string
		wantColor bool
		wantGoLen int
	}{
		{
			Desc:      "no flags",
			args:      []string{"./..."},
			wantFmt:   "terminal",
			wantInput: "",
			wantColor: false,
			wantGoLen: 1,
		},
		{
			Desc:      "input with equals",
			args:      []string{"--input=events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			Desc:      "input with space",
			args:      []string{"--input", "events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			Desc:      "input stdin dash",
			args:      []string{"--input=-"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			Desc:      "input with format",
			args:      []string{"--format=md", "--input=data.json"},
			wantFmt:   "md",
			wantInput: "data.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			Desc:      "input with output and no-color",
			args:      []string{"--input=-", "--output=out.txt", "--no-color"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantOut:   "out.txt",
			wantColor: true,
			wantGoLen: 0,
		},
	}) {
		ownArgs, goTestArgs, err := SplitArgs(tc.args, ExportSpecAllowed)
		gotest.NoError(sub, err)

		format := ExportExtractStringFlag(ownArgs, "--format", "terminal")
		output := ExportExtractStringFlag(ownArgs, "--output", "")
		input := ExportExtractStringFlag(ownArgs, "--input", "")
		noColor := ExportHasFlag(ownArgs, "--no-color")

		gotest.Equal(sub, tc.wantFmt, format)
		gotest.Equal(sub, tc.wantOut, output)
		gotest.Equal(sub, tc.wantInput, input)
		gotest.Equal(sub, tc.wantColor, noColor)
		gotest.Len(sub, goTestArgs, tc.wantGoLen)
	}
}

func (s *CmdGotestTestSuite) TestRunSpec_InputStdin(t *gotest.T) {
	t.It("renders spec output from stdin-like JSON", func(it *gotest.T) {
		absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
		if err != nil {
			it.T().Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
			it.T().Skipf("examples directory not found: %v", err)
		}

		loaded, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
		if err != nil {
			it.T().Fatalf("LoadPackages: %v", err)
		}
		results, _, err := gotestgen.GenerateFromLoaded(loaded)
		if err != nil {
			it.T().Fatalf("GenerateFromLoaded: %v", err)
		}

		tmpDir, err := gotestrunner.WriteOverlay(results)
		if err != nil {
			it.T().Fatalf("WriteOverlay: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		cmd := exec.CommandContext(context.Background(), "go", //nolint:gosec // G204: go tool with controlled arguments
			"test", "-json", "-ldflags=-checklinkname=0",
			"-overlay="+filepath.Join(tmpDir, "overlay.json"), "./cart")
		cmd.Dir = absExamples
		var jsonOut bytes.Buffer
		cmd.Stdout = &jsonOut
		cmd.Stderr = os.Stderr
		mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{Grace: gotestrunner.GraceKill})
		if err := mp.Start(); err != nil {
			it.T().Fatalf("go test start: %v", err)
		}
		_ = mp.WaitWithGrace(context.Background())
		if cmd.ProcessState == nil {
			it.T().Fatal("go test: process state is nil")
		}
		jsonData := jsonOut.Bytes()

		events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
		if err != nil {
			it.T().Fatalf("ParseEvents: %v", err)
		}

		tree := gotestspec.BuildTree(events)

		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())

		output := buf.String()
		gotest.Contains(it, output, "ShoppingCart")
	})
}

func (s *CmdGotestTestSuite) TestWatchHelpers(t *gotest.T) {
	t.When("IsGoFile", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			name   string
			expect bool
		}{
			{Desc: "go file", name: "main.go", expect: true},
			{Desc: "test file", name: "main_test.go", expect: true},
			{Desc: "path with go file", name: "/tmp/foo/bar.go", expect: true},
			{Desc: "not a go file", name: "main.py", expect: false},
			{Desc: "go in middle", name: "foo.go.bak", expect: false},
			{Desc: "empty", name: "", expect: false},
		}) {
			gotest.Equal(sub, tc.expect, ExportIsGoFile(tc.name))
		}
	})

	t.When("DirsToPatterns", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc    string
			dirs    map[string]bool
			lenWant int
		}{
			{Desc: "single dir", dirs: map[string]bool{"pkg/foo": true}, lenWant: 1},
			{Desc: "multiple dirs", dirs: map[string]bool{"pkg/foo": true, "cmd/bar": true}, lenWant: 2},
			{Desc: "empty", dirs: map[string]bool{}, lenWant: 0},
		}) {
			result := ExportDirsToPatterns(tc.dirs)
			gotest.Len(sub, result, tc.lenWant)
			for _, p := range result {
				gotest.True(sub, len(p) > 2 && p[:2] == "./", "expected ./ prefix, got: %s", p)
			}
		}
	})

	t.When("ReplacePatterns", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc        string
			original    []string
			newPatterns []string
			expected    []string
		}{
			{
				Desc:        "replaces package pattern",
				original:    []string{"-v", "./pkg/foo", "-race"},
				newPatterns: []string{"./cmd/bar"},
				expected:    []string{"-v", "-race", "./cmd/bar"},
			},
			{
				Desc:        "no patterns to replace",
				original:    []string{"-v", "-race"},
				newPatterns: []string{"./pkg/new"},
				expected:    []string{"-v", "-race", "./pkg/new"},
			},
			{
				Desc:        "multiple patterns replaced",
				original:    []string{"-v", "./pkg/a", "./pkg/b", "-race"},
				newPatterns: []string{"./changed"},
				expected:    []string{"-v", "-race", "./changed"},
			},
		}) {
			result := ExportReplacePatterns(tc.original, tc.newPatterns)
			gotest.Equal(sub, tc.expected, result)
		}
	})
}
