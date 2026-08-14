package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CmdGotestTestSuite tests CLI argument parsing, subcommands,
// discovery, spec rendering, and code generation.
//
//nolint:lifecycle-pair // BeforeAll's binary lives under t.TempDir(), which the framework removes automatically
type CmdGotestTestSuite struct {
	binary   string
	repoRoot string
}

func (s *CmdGotestTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *CmdGotestTestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.repoRoot = absRoot

	binDir := t.TempDir()
	binaryName := "gotest"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	s.binary = filepath.Join(binDir, binaryName)
	cmd := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	cmd.Dir = absRoot
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "build gotest binary: %s", string(out))
}

// runCLI runs the built gotest binary from the repo root and returns its
// combined stdout+stderr output.
func (s *CmdGotestTestSuite) runCLI(t *gotest.T, args ...string) string {
	out, _ := s.runCLIExit(t, args...)
	return out
}

// runCLIExit runs the built gotest binary from the repo root and returns its
// combined stdout+stderr output along with its exit code.
func (s *CmdGotestTestSuite) runCLIExit(t *gotest.T, args ...string) (string, int) {
	cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.repoRoot
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, out)
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return string(out), code
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

// specTableEntries extracts `name` from rows shaped `| `name` | ... |` within the
// doc section starting at heading until the next heading of the same level.
func specTableEntries(doc, heading string) map[string]bool {
	return specTableEntriesUntil(doc, heading, "\n### ")
}

// specTableEntriesUntil is specTableEntries with an explicit section
// terminator, for sections delimited by a different heading level.
func specTableEntriesUntil(doc, heading, next string) map[string]bool {
	start := strings.Index(doc, heading)
	if start < 0 {
		return nil
	}
	section := doc[start:]
	if end := strings.Index(section[len(heading):], next); end >= 0 {
		section = section[:len(heading)+end]
	}
	entries := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([^`=<]+)").FindAllStringSubmatch(section, -1) {
		entries[strings.TrimSpace(m[1])] = true
	}
	return entries
}

// TestCLISurfaceMatchesSpec is a drift guard: the CLI tables in docs/design/spec.md
// must stay in sync with the actual subcommand and flag registries.
func (s *CmdGotestTestSuite) TestCLISurfaceMatchesSpec(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "design", "spec.md"))
	gotest.NoError(t, err)
	doc := string(data)

	t.When("comparing the Subcommands table", func(w *gotest.T) {
		documented := specTableEntries(doc, "### Subcommands")
		w.It("documents every registered subcommand", func(it *gotest.T) {
			for cmd := range ExportKnownSubcommands {
				gotest.True(it, documented[cmd], "subcommand %q missing from spec.md", cmd)
			}
		})
		w.It("documents no phantom subcommands", func(it *gotest.T) {
			for cmd := range documented {
				gotest.True(it, ExportKnownSubcommands[cmd], "spec.md documents unknown subcommand %q", cmd)
			}
		})
	})

	t.When("comparing the Flags table", func(w *gotest.T) {
		documented := specTableEntries(doc, "### Flags")
		w.It("documents every registered --flag", func(it *gotest.T) {
			for flag := range ExportGotestFlags {
				gotest.True(it, documented[flag], "flag %q missing from spec.md", flag)
			}
		})
		w.It("documents no phantom flags", func(it *gotest.T) {
			for flag := range documented {
				_, known := ExportGotestFlags[flag]
				gotest.True(it, known, "spec.md documents unknown flag %q", flag)
			}
		})
	})

	t.When("comparing the Linter rule tables", func(w *gotest.T) {
		documented := specTableEntriesUntil(doc, "## Linter", "\n## ")
		w.It("documents every registered lint rule", func(it *gotest.T) {
			for _, rule := range lint.RuleIDs() {
				gotest.True(it, documented[string(rule)], "lint rule %q missing from spec.md", rule)
			}
		})
		w.It("documents no phantom lint rules", func(it *gotest.T) {
			for id := range documented {
				gotest.True(it, lint.Known(lint.Rule(id)), "spec.md documents unknown lint rule %q", id)
			}
		})
	})
}

// CmdEnvTestSuite is deliberately sequential: Setenv is illegal in parallel tests.
type CmdEnvTestSuite struct{}

func (s *CmdEnvTestSuite) TestDetectCIEnv(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		gotestCI string
		ci       string
		expect   bool
	}{
		{Desc: "unset both", gotestCI: "", ci: "", expect: false},
		{Desc: "GOTEST_CI=1", gotestCI: "1", ci: "", expect: true},
		{Desc: "GOTEST_CI=true", gotestCI: "true", ci: "", expect: true},
		{Desc: "typo'd opt-in stays on", gotestCI: "yes", ci: "", expect: true},
		{Desc: "GOTEST_CI=0 opts out of CI env", gotestCI: "0", ci: "true", expect: false},
		{Desc: "GOTEST_CI=false opts out", gotestCI: "false", ci: "1", expect: false},
		{Desc: "CI set", gotestCI: "", ci: "true", expect: true},
		{Desc: "CI=false is not CI", gotestCI: "", ci: "false", expect: false},
		{Desc: "CI=0 is not CI", gotestCI: "", ci: "0", expect: false},
	}) {
		sub.Setenv("GOTEST_CI", tc.gotestCI)
		sub.Setenv("CI", tc.ci)
		gotest.Equal(sub, tc.expect, ExportDetectCIEnv())
	}
}

func (s *CmdGotestTestSuite) TestScaffoldRejectsUnknownFlags(t *gotest.T) {
	code := ExportRunScaffold(Invocation{Args: []string{"--contract", "io.Reader"}})
	gotest.Equal(t, 2, code)
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

func (s *CmdGotestTestSuite) TestParseCompileParallelFlag(t *gotest.T) {
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

func (s *CmdGotestTestSuite) TestParseExecFlags_HarvestSeeds(t *gotest.T) {
	falsePtr := false
	for sub, tc := range gotest.Each(t, []struct {
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
		gotest.NoError(it, err, "%v", err)
		if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
			it.Skipf("examples directory not found: %v", err)
		}

		loadResults, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
		gotest.NoError(it, err, "LoadPackages: %v", err)
		gotest.NotEmpty(it, loadResults, "expected at least one load result")

		out := ExportDiscoverOutput{}
		c := gotestgen.NewCollector()
		for _, lr := range loadResults {
			pkgEntry := ExportDiscoverPackage{
				ImportPath: lr.PkgPath,
				Dir:        lr.PkgDir,
			}

			collect := func(pkg *packages.Package) {
				result := c.CollectSuiteSpecs(pkg)
				var collectorErrs []string
				for _, cerr := range result.Errs {
					collectorErrs = append(collectorErrs, cerr.Err.Error())
				}
				gotest.Empty(it, collectorErrs, "collector errors")
				for _, suite := range result.Suites {
					pkgEntry.Suites = append(pkgEntry.Suites, ExportBuildDiscoverSuite(suite))
				}
			}
			if lr.Ptest != nil {
				collect(lr.Ptest)
			}
			if lr.Pxtest != nil {
				collect(lr.Pxtest)
			}

			out.Packages = append(out.Packages, pkgEntry)
		}

		gotest.Len(it, out.Packages, 1)

		pkg := out.Packages[0]
		gotest.Equal(it, "github.com/mvrahden/go-test/examples/cart", pkg.ImportPath)
		gotest.True(it, filepath.IsAbs(pkg.Dir), "dir should be absolute, got %q", pkg.Dir)

		gotest.Len(it, pkg.Suites, 4)

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

		gotest.Len(it, st.Methods, 9)
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
		gotest.Len(it, sx.Methods, 2)
		gotest.Equal(it, "TestAddItem", sx.Methods[0].Name)
		gotest.Equal(it, "TestRemoveItem", sx.Methods[1].Name)

		// Verify pxtest PricingExtTestSuite (fixture-bound)
		px := suiteByNameAndFile["PricingExtTestSuite:pricing_ext_suite_test.go"]
		gotest.Equal(it, "PricingExtTestSuite", px.Name)

		// Verify JSON serialization roundtrip
		data, err := json.Marshal(out)
		gotest.NoError(it, err, "json.Marshal: %v", err)
		var roundtrip ExportDiscoverOutput
		gotest.NoError(it, json.Unmarshal(data, &roundtrip))
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
		{
			Desc:     "suite violation with position",
			v:        FocusViolation{SuiteName: "F_MyTestSuite", Pos: "pkg/user/user_test.go:12"},
			expected: "  pkg/user/user_test.go:12  type F_MyTestSuite",
		},
		{
			Desc:     "method violation with position",
			v:        FocusViolation{SuiteName: "MyTestSuite", MethodName: "F_TestSomething", Pos: "pkg/user/user_test.go:28"},
			expected: "  pkg/user/user_test.go:28  MyTestSuite.F_TestSomething",
		},
	}) {
		gotest.Equal(sub, tc.expected, tc.v.String())
	}
}

func (s *CmdGotestTestSuite) TestGenerateOverlay(t *gotest.T) {
	t.When("suites are present", func(w *gotest.T) {
		w.It("produces valid overlay JSON", func(it *gotest.T) {
			absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
			gotest.NoError(it, err, "%v", err)
			if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
				it.Skipf("examples directory not found: %v", err)
			}

			loaded, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
			gotest.NoError(it, err, "LoadPackages: %v", err)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err, "GenerateFromLoaded: %v", err)
			gotest.NotEmpty(it, results, "expected at least one generate result")

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err, "WriteOverlay: %v", err)
			defer os.RemoveAll(tmpDir)

			overlayFile := filepath.Join(tmpDir, "overlay.json")
			_, err = os.Stat(overlayFile)
			gotest.NoError(it, err)

			data, err := os.ReadFile(overlayFile)
			gotest.NoError(it, err, "reading overlay.json: %v", err)
			var overlayContent struct {
				Replace map[string]string `json:"Replace"`
			}
			gotest.NoError(it, json.Unmarshal(data, &overlayContent), "overlay.json is not valid JSON")
			gotest.NotEmpty(it, overlayContent.Replace, "overlay.json Replace map is empty")
		})
	})

	t.When("no suites", func(w *gotest.T) {
		w.It("returns empty results for package without suites", func(it *gotest.T) {
			tmpDir, err := os.MkdirTemp("", "overlay-test-nosuite-*")
			gotest.NoError(it, err, "%v", err)
			defer os.RemoveAll(tmpDir)

			gotest.NoError(it, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module nosuite\n\ngo 1.24\n"), 0600))
			gotest.NoError(it, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0600))

			loaded, _, err := gotestgen.LoadPackages([]string{tmpDir}, nil)
			gotest.NoError(it, err, "LoadPackages: %v", err)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err, "GenerateFromLoaded: %v", err)

			var allResults gotestgen.GenerateResults
			allResults = append(allResults, results...)
			if len(allResults) != 0 {
				it.Skipf("expected 0 results for package without suites, got %d (package may have test suites)", len(allResults))
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
		gotest.NoError(it, err, "%v", err)
		if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
			it.Skipf("examples directory not found: %v", err)
		}

		loaded, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
		gotest.NoError(it, err, "LoadPackages: %v", err)
		results, _, err := gotestgen.GenerateFromLoaded(loaded)
		gotest.NoError(it, err, "GenerateFromLoaded: %v", err)

		tmpDir, err := gotestrunner.WriteOverlay(results)
		gotest.NoError(it, err, "WriteOverlay: %v", err)
		defer os.RemoveAll(tmpDir)

		cmd := exec.CommandContext(context.Background(), "go", //nolint:gosec // G204: go tool with controlled arguments
			"test", "-json", "-ldflags=-checklinkname=0",
			"-overlay="+filepath.Join(tmpDir, "overlay.json"), "./cart")
		cmd.Dir = absExamples
		var jsonOut bytes.Buffer
		cmd.Stdout = &jsonOut
		cmd.Stderr = os.Stderr
		mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{Grace: gotestrunner.GraceKill})
		gotest.NoError(it, mp.Start(), "go test start")
		_ = mp.WaitWithGrace(context.Background())
		gotest.NotZero(it, cmd.ProcessState, "go test: process state is nil")
		jsonData := jsonOut.Bytes()

		events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
		gotest.NoError(it, err, "ParseEvents: %v", err)

		tree := gotestspec.BuildTree(events)

		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())

		output := buf.String()
		gotest.Contains(it, output, "ShoppingCart")
	})
}

func (s *CmdGotestTestSuite) TestInputModesShareOneExitRule(t *gotest.T) {
	// One failing and one passing stream, saved the way CI replays them. In a
	// pipe without pipefail the exit code of the rendering command is the only
	// verdict CI ever sees — and `spec --input` used to return 0 on anything.
	failingStream := `{"Action":"run","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"output","Package":"example.com/pkg","Test":"TestBoom","Output":"--- FAIL: TestBoom (0.00s)\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"fail","Package":"example.com/pkg"}
`
	greenStream := `{"Action":"run","Package":"example.com/pkg","Test":"TestOK"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestOK"}
{"Action":"pass","Package":"example.com/pkg"}
`

	writeStream := func(w *gotest.T, content string) string {
		path := filepath.Join(w.TempDir(), "events.json")
		gotest.NoError(w, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	t.When("the stream contains a failure", func(w *gotest.T) {
		input := writeStream(w, failingStream)
		out := filepath.Join(w.TempDir(), "out.txt")

		w.It("spec --input exits nonzero", func(it *gotest.T) {
			gotest.Equal(it, 1, ExportRunSpecFromInput(input, "terminal", out, true, false))
		})

		w.It("summary --input agrees", func(it *gotest.T) {
			gotest.Equal(it, 1, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, false))
		})
	})

	t.When("the stream is clean", func(w *gotest.T) {
		input := writeStream(w, greenStream)
		out := filepath.Join(w.TempDir(), "out.txt")

		w.It("both exit zero", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "terminal", out, true, false))
			gotest.Equal(it, 0, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, false))
		})
	})
}

// TestRenderOnlySeparatesVerdictFromRendering covers the escape hatch for
// clients that render a stream rather than gate on it. The exit code of
// `--input` carries two answers at once — "did the tests pass" and "did I
// render" — and a renderer only ever needed the second. --render-only drops the
// first without ever hiding the second.
func (s *CmdGotestTestSuite) TestRenderOnlySeparatesVerdictFromRendering(t *gotest.T) {
	failingStream := `{"Action":"run","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"output","Package":"example.com/pkg","Test":"TestBoom","Output":"--- FAIL: TestBoom (0.00s)\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"fail","Package":"example.com/pkg"}
`

	writeStream := func(w *gotest.T, content string) string {
		path := filepath.Join(w.TempDir(), "events.json")
		gotest.NoError(w, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	t.When("a failing stream is rendered with --render-only", func(w *gotest.T) {
		input := writeStream(w, failingStream)
		out := filepath.Join(w.TempDir(), "out.txt")

		w.It("spec reports success, because rendering succeeded", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "terminal", out, true, true))
		})

		w.It("summary agrees, keeping the two input modes on one rule", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, true))
		})

		w.It("still renders the failure into the output", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "terminal", out, true, true))
			data, err := os.ReadFile(out)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "Boom")
		})
	})

	t.When("the input cannot be read", func(w *gotest.T) {
		missing := filepath.Join(w.TempDir(), "absent.json")
		out := filepath.Join(w.TempDir(), "out.txt")

		// The whole point of the flag is to suppress a verdict about the tests,
		// never a report that the command itself failed.
		w.It("spec still fails, because nothing was rendered", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSpecFromInput(missing, "terminal", out, true, true))
		})

		w.It("summary still fails too", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSummaryFromInput(missing, "terminal", out, "", true, false, true))
		})
	})

	t.When("--render-only is passed without --input", func(w *gotest.T) {
		// Suppressing the verdict of a real run would turn a red pipeline green,
		// so the flag is refused outside the replay path it was built for.
		w.It("spec rejects it as a usage error", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSpec(Invocation{Args: []string{"--render-only", "./..."}}))
		})

		w.It("summary rejects it as a usage error", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSummary(Invocation{Args: []string{"--render-only", "./..."}}))
		})
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

func (s *CmdGotestTestSuite) TestFuzzSubcommand(t *gotest.T) {
	t.It("reports when no fuzz targets exist", func(it *gotest.T) {
		out, code := s.runCLIExit(it, "fuzz", "./internal/protocol")
		gotest.Contains(it, out, "no fuzz targets found")
		gotest.Equal(it, 0, code)
	})
}

// runScaffoldFuzzCLI writes files (module + a single "codec.go" source) to
// an isolated temp module and runs "gotest scaffold --fuzz" from inside it,
// so the command's writeScaffoldFile output never touches the real repo.
func (s *CmdGotestTestSuite) runScaffoldFuzzCLI(t *gotest.T, codecSrc, funcName string) (string, int, string) { //nolint:gocritic // hugeParam: test helper
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fuzzscaffold\n\ngo 1.24\n"), 0644)) //nolint:gosec // G306: throwaway test module
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "codec"), 0755))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "codec", "codec.go"), []byte(codecSrc), 0644)) //nolint:gosec // G306: throwaway test module

	cmd := exec.Command(s.binary, "scaffold", "--fuzz", "./codec."+funcName) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, out)
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return string(out), code, dir
}

func (s *CmdGotestTestSuite) TestScaffoldFuzzSubcommand(t *gotest.T) {
	t.It("generates a round-trip skeleton for a found inverse pair", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

func Encode(s string) ([]byte, error) { return []byte(s), nil }
func Decode(b []byte) (string, error) { return string(b), nil }
`, "Encode")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "Generated: codec/encode_fuzz_test.go")

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "encode_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.Contains(it, src, "gotest.Fuzz(")
		gotest.Contains(it, src, "Encode")
		gotest.Contains(it, src, "Decode")
		gotest.Contains(it, src, "gotest.Equal(t, in, decoded) // round-trip property")
	})

	t.It("falls back to a crash-safety skeleton when no inverse pair exists", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

func Render(n int) string { return "" }
`, "Render")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "no inverse pair found for Render — generated crash-safety skeleton")
		gotest.Contains(it, out, "Generated: codec/render_fuzz_test.go")

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "render_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.Contains(it, src, "gotest.Fuzz(")
		gotest.Contains(it, src, "Render(in)")
	})

	t.It("scaffolds a real skeleton for a codec-fuzzable struct parameter", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

type Config struct{ Name string }

func ApplyConfig(c Config) string { return c.Name }
`, "ApplyConfig")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "no inverse pair found for ApplyConfig — generated crash-safety skeleton")
		gotest.Contains(it, out, "Generated: codec/apply_config_fuzz_test.go")

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "apply_config_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.Contains(it, src, "gotest.Fuzz(")
		gotest.Contains(it, src, "f.Add(Config{})")
	})

	t.It("falls back to a TODO stub carrying the codec emitter's rejection", func(it *gotest.T) {
		out, code, dir := s.runScaffoldFuzzCLI(it, `package codec

func ApplyOptions(opts map[string]string) string { return opts["name"] }
`, "ApplyOptions")
		gotest.Equal(it, 0, code)
		gotest.Contains(it, out, "cannot fuzz map[string]string for ApplyOptions — generated TODO stub: ")
		gotest.Contains(it, out, "maps have no canonical encoding")
		gotest.Contains(it, out, "Generated: codec/apply_options_fuzz_test.go")

		generated, err := os.ReadFile(filepath.Join(dir, "codec", "apply_options_fuzz_test.go"))
		gotest.NoError(it, err)
		src := string(generated)
		gotest.NotContains(it, src, "gotest.Fuzz(")
		gotest.Contains(it, src, "maps have no canonical encoding")
	})
}
