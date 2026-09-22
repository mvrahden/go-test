package main_test

import (
	"time"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FlagParsingTestSuite covers how the CLI reads its command line: subcommands, flags, their defaults and the package patterns.
type FlagParsingTestSuite struct{}

func (s *FlagParsingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FlagParsingTestSuite) TestDefaultArgs(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestSplitArgs(t *gotest.T) {
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
		{Desc: "watch: spec flag", inArgs: []string{"--spec", "-v", "./..."}, allowed: ExportWatchAllowed, expectOwn: []string{"--spec"}, expectGoTest: []string{"-v", "./..."}},
		{Desc: "watch: json flag", inArgs: []string{"-json", "./pkg/..."}, allowed: ExportWatchAllowed, expectOwn: nil, expectGoTest: []string{"-json", "./pkg/..."}},
		{Desc: "watch: debounce with json", inArgs: []string{"--debounce=500ms", "-json", "./..."}, allowed: ExportWatchAllowed, expectOwn: []string{"--debounce=500ms"}, expectGoTest: []string{"-json", "./..."}},
		{Desc: "watch: debug and ci", inArgs: []string{"--debug", "--ci", "-v", "./..."}, allowed: ExportWatchAllowed, expectOwn: []string{"--debug", "--ci"}, expectGoTest: []string{"-v", "./..."}},
		{Desc: "timeout flag with equals", inArgs: []string{"--timeout=15m", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--timeout=15m"}, expectGoTest: []string{"-v"}},
		{Desc: "timeout flag with space", inArgs: []string{"--timeout", "15m", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--timeout", "15m"}, expectGoTest: []string{"-v"}},
		{Desc: "no-harvest allowed for test", inArgs: []string{"--no-harvest", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--no-harvest"}, expectGoTest: []string{"-v"}},
		{Desc: "no-harvest allowed for fuzz", inArgs: []string{"--no-harvest", "--for=1m"}, allowed: ExportFuzzAllowed, expectOwn: []string{"--no-harvest", "--for=1m"}, expectGoTest: nil},
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

func (s *FlagParsingTestSuite) TestVetFlag(t *gotest.T) {
	t.It("forwards -vet to go test", func(it *gotest.T) {
		own, goTest, err := SplitArgs([]string{"-vet=off", "./..."}, ExportTestAllowed)
		gotest.NoError(it, err)
		gotest.Empty(it, own)
		gotest.Equal(it, []string{"-vet=off", "./..."}, goTest)
	})

	t.It("still rejects --vet, which is nobody's flag", func(it *gotest.T) {
		_, _, err := SplitArgs([]string{"--vet", "./..."}, ExportTestAllowed)
		gotest.ErrorContains(it, err, "unknown flag: --vet")
	})
}

func (s *FlagParsingTestSuite) TestParseSubcommand(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestPackagePatterns(t *gotest.T) {
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
			{Desc: "space-separated flag value with a slash is not a package", args: []string{"./pkg/a", "-bench", "^BenchmarkFooTestSuite$/^BenchmarkParse$"}, expected: []string{"./pkg/a"}},
			{Desc: "space-separated -run value with a slash is not a package", args: []string{"-run", "TestFoo/sub", "./pkg/a"}, expected: []string{"./pkg/a"}},
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

func (s *FlagParsingTestSuite) TestParseMinFlag(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestParseParallelFlag(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestParseCompileParallelFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc      string
		args      []string
		expect    int
		expectErr bool
	}{
		{Desc: "no flag", args: []string{"--debug"}, expect: 0},
		{Desc: "equals syntax", args: []string{"--compile-parallel=4"}, expect: 4},
		{Desc: "space syntax", args: []string{"--compile-parallel", "2"}, expect: 2},
		{Desc: "empty args", args: nil, expect: 0},
		{Desc: "invalid value", args: []string{"--compile-parallel=abc"}, expectErr: true},
		{Desc: "zero value", args: []string{"--compile-parallel=0"}, expectErr: true},
		{Desc: "negative value", args: []string{"--compile-parallel=-1"}, expectErr: true},
	}) {
		got, err := ExportParseCompileParallelFlag(tc.args)
		if tc.expectErr {
			gotest.Error(sub, err, "expected error")
		} else {
			gotest.NoError(sub, err)
			gotest.Equal(sub, tc.expect, got)
		}
	}
}

func (s *FlagParsingTestSuite) TestParseSetupTimeoutFlag(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestParseExecFlags_HarvestSeeds(t *gotest.T) {
	falsePtr := false
	for sub, tc := range gotest.Each(t, []struct { //nolint:gocritic // rangeValCopy: intentional
		Desc    string
		ownArgs []string
		cfg     config.ProjectConfig
		expect  bool
	}{
		{Desc: "default: no flag, no config", ownArgs: nil, cfg: config.ProjectConfig{}, expect: true},
		{Desc: "--no-harvest disables it", ownArgs: []string{"--no-harvest"}, cfg: config.ProjectConfig{}, expect: false},
		{Desc: "config fuzz.harvest=false disables it", ownArgs: nil, cfg: config.ProjectConfig{Fuzz: config.FuzzConfig{Harvest: &falsePtr}}, expect: false},
		{Desc: "flag and config both disabling stays disabled", ownArgs: []string{"--no-harvest"}, cfg: config.ProjectConfig{Fuzz: config.FuzzConfig{Harvest: &falsePtr}}, expect: false},
	}) {
		got, err := ExportParseExecFlags(tc.ownArgs, nil, &tc.cfg)
		gotest.NoError(sub, err)
		gotest.Equal(sub, tc.expect, got.HarvestSeeds)
	}
}

func (s *FlagParsingTestSuite) TestParseDebounceFlag(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestParseGlobalTimeoutFlag(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestResolveGlobalTimeout(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestSpecFlagParsing(t *gotest.T) {
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

func (s *FlagParsingTestSuite) TestScaffoldRejectsUnknownFlags(t *gotest.T) {
	code := ExportRunScaffold(Invocation{Args: []string{"--contract", "io.Reader"}})
	gotest.Equal(t, 2, code)
}
