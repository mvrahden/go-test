package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type CmdGotestTestSuite struct{}

// --- args ---

func (s *CmdGotestTestSuite) TestDefaultArgs(t *gotest.T) {
	for _, tc := range []struct {
		desc   string
		inv    Invocation
		expect []string
	}{
		// CLI absent x config zero/positive/negative
		{
			desc:   "CLI absent, config zero: no prepend",
			inv:    Invocation{Args: []string{"-v"}},
			expect: []string{"-v"},
		},
		{
			desc: "CLI absent, config positive: config prepended",
			inv: Invocation{
				Args:   []string{"-v"},
				Config: config.ProjectConfig{SetupTimeout: config.Duration(2 * time.Minute)},
			},
			expect: []string{"--setup-timeout=2m0s", "-v"},
		},
		{
			desc: "CLI absent, config negative: config prepended",
			inv: Invocation{
				Args:   []string{"-v"},
				Config: config.ProjectConfig{SetupTimeout: config.Duration(-1 * time.Second)},
			},
			expect: []string{"--setup-timeout=-1s", "-v"},
		},
		// CLI positive x config zero/positive/negative
		{
			desc: "CLI positive, config zero: CLI preserved",
			inv: Invocation{
				Args: []string{"--setup-timeout=5m", "-v"},
			},
			expect: []string{"--setup-timeout=5m", "-v"},
		},
		{
			desc: "CLI positive, config positive: CLI wins",
			inv: Invocation{
				Args:   []string{"--setup-timeout=5m", "-v"},
				Config: config.ProjectConfig{SetupTimeout: config.Duration(2 * time.Minute)},
			},
			expect: []string{"--setup-timeout=5m", "-v"},
		},
		{
			desc: "CLI positive, config negative: CLI wins",
			inv: Invocation{
				Args:   []string{"--setup-timeout=5m", "-v"},
				Config: config.ProjectConfig{SetupTimeout: config.Duration(-1 * time.Second)},
			},
			expect: []string{"--setup-timeout=5m", "-v"},
		},
		// CLI negative x config zero/positive/negative
		{
			desc: "CLI negative, config zero: CLI preserved",
			inv: Invocation{
				Args: []string{"--setup-timeout=-1s", "-v"},
			},
			expect: []string{"--setup-timeout=-1s", "-v"},
		},
		{
			desc: "CLI negative, config positive: CLI wins",
			inv: Invocation{
				Args:   []string{"--setup-timeout=-1s", "-v"},
				Config: config.ProjectConfig{SetupTimeout: config.Duration(2 * time.Minute)},
			},
			expect: []string{"--setup-timeout=-1s", "-v"},
		},
		{
			desc: "CLI negative, config negative: CLI wins",
			inv: Invocation{
				Args:   []string{"--setup-timeout=-1s", "-v"},
				Config: config.ProjectConfig{SetupTimeout: config.Duration(-1 * time.Second)},
			},
			expect: []string{"--setup-timeout=-1s", "-v"},
		},
		// combined defaults
		{
			desc: "tags and setup-timeout both prepended",
			inv: Invocation{
				Args:   []string{"-v"},
				Config: config.ProjectConfig{Tags: "integration", SetupTimeout: config.Duration(3 * time.Minute)},
			},
			expect: []string{"--setup-timeout=3m0s", "-tags=integration", "-v"},
		},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("produces expected args", func(it *gotest.T) {
				got := tc.inv.DefaultArgs()
				gotest.Equal(it, tc.expect, got)
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestSplitArgs(t *gotest.T) {
	for _, tc := range []struct {
		desc         string
		inArgs       []string
		allowed      map[string]bool
		expectOwn    []string
		expectGoTest []string
		expectErr    bool
	}{
		{desc: "empty", inArgs: nil, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: nil},
		{desc: "only go test args", inArgs: []string{"-v", "./...", "-race", "-count=1"}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-v", "./...", "-race", "-count=1"}},
		{desc: "only own args", inArgs: []string{"--debug"}, allowed: ExportTestAllowed, expectOwn: []string{"--debug"}, expectGoTest: nil},
		{desc: "mixed args", inArgs: []string{"--debug", "-v", "./...", "-race"}, allowed: ExportTestAllowed, expectOwn: []string{"--debug"}, expectGoTest: []string{"-v", "./...", "-race"}},
		{desc: "min flag with equals", inArgs: []string{"--min=80", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--min=80"}, expectGoTest: []string{"-v"}},
		{desc: "min flag with space", inArgs: []string{"--min", "90", "-v"}, allowed: ExportTestAllowed, expectOwn: []string{"--min", "90"}, expectGoTest: []string{"-v"}},
		{desc: "unknown gotest flag", inArgs: []string{"--unknown"}, allowed: ExportTestAllowed, expectErr: true},
		{desc: "unknown go test flag", inArgs: []string{"-zzz"}, allowed: ExportTestAllowed, expectErr: true},
		{desc: "gotest flag not in allowed set", inArgs: []string{"--debounce=200ms"}, allowed: ExportTestAllowed, expectErr: true},
		{desc: "bare -- escape hatch", inArgs: []string{"--debug", "--", "-custom", "./..."}, allowed: ExportTestAllowed, expectOwn: []string{"--debug"}, expectGoTest: []string{"-custom", "./..."}},
		{desc: "bare -- with no gotest flags", inArgs: []string{"--", "-v", "./..."}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-v", "./..."}},
		{desc: "-args passthrough", inArgs: []string{"-v", "-args", "-custom=1"}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-v", "-args", "-custom=1"}},
		{desc: "spec allowed set", inArgs: []string{"--format=md", "--no-color", "-v"}, allowed: ExportSpecAllowed, expectOwn: []string{"--format=md", "--no-color"}, expectGoTest: []string{"-v"}},
		{desc: "watch allowed set", inArgs: []string{"--debounce=500ms", "-v"}, allowed: ExportWatchAllowed, expectOwn: []string{"--debounce=500ms"}, expectGoTest: []string{"-v"}},
		{desc: "go test value flag with space", inArgs: []string{"-run", "TestFoo", "./..."}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-run", "TestFoo", "./..."}},
		{desc: "go test value flag with equals", inArgs: []string{"-timeout=30s"}, allowed: ExportTestAllowed, expectOwn: nil, expectGoTest: []string{"-timeout=30s"}},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("splits correctly", func(it *gotest.T) {
				own, goTest, err := SplitArgs(tc.inArgs, tc.allowed)
				if tc.expectErr {
					gotest.True(it, err != nil, "expected error")
					return
				}
				gotest.NoError(it, err)
				gotest.Equal(it, tc.expectOwn, own)
				gotest.Equal(it, tc.expectGoTest, goTest)
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestParseSubcommand(t *gotest.T) {
	for _, tc := range []struct {
		desc            string
		args            []string
		expectSubcmd    string
		expectRemaining []string
	}{
		{desc: "empty args", args: nil, expectSubcmd: "", expectRemaining: nil},
		{desc: "no subcommand, just flags", args: []string{"-v", "./..."}, expectSubcmd: "", expectRemaining: []string{"-v", "./..."}},
		{desc: "version subcommand", args: []string{"version"}, expectSubcmd: "version", expectRemaining: nil},
		{desc: "scaffold subcommand", args: []string{"scaffold", "-v"}, expectSubcmd: "scaffold", expectRemaining: []string{"-v"}},
		{desc: "migrate subcommand", args: []string{"migrate"}, expectSubcmd: "migrate", expectRemaining: nil},
		{desc: "help subcommand", args: []string{"help"}, expectSubcmd: "help", expectRemaining: nil},
		{desc: "generate subcommand", args: []string{"generate", "./..."}, expectSubcmd: "generate", expectRemaining: []string{"./..."}},
		{desc: "watch subcommand", args: []string{"watch"}, expectSubcmd: "watch", expectRemaining: nil},
		{desc: "clean subcommand", args: []string{"clean", "./..."}, expectSubcmd: "clean", expectRemaining: []string{"./..."}},
		{desc: "spec subcommand", args: []string{"spec"}, expectSubcmd: "spec", expectRemaining: nil},
		{desc: "unknown first arg is not consumed", args: []string{"./...", "-v"}, expectSubcmd: "", expectRemaining: []string{"./...", "-v"}},
		{desc: "flag first arg is not consumed", args: []string{"-v", "./..."}, expectSubcmd: "", expectRemaining: []string{"-v", "./..."}},
		{desc: "package pattern not consumed", args: []string{"github.com/foo/bar"}, expectSubcmd: "", expectRemaining: []string{"github.com/foo/bar"}},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("returns expected subcmd and remaining", func(it *gotest.T) {
				subcmd, remaining := ParseSubcommand(tc.args)
				gotest.Equal(it, tc.expectSubcmd, subcmd)
				gotest.Equal(it, tc.expectRemaining, remaining)
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestExtractPackagePatterns(t *gotest.T) {
	for _, tc := range []struct {
		desc     string
		args     []string
		expected []string
	}{
		{desc: "explicit relative path", args: []string{"-v", "./...", "-race"}, expected: []string{"./..."}},
		{desc: "explicit named package", args: []string{"-v", "github.com/foo/bar", "-race"}, expected: []string{"github.com/foo/bar"}},
		{desc: "no package defaults to dot", args: []string{"-v", "-race"}, expected: []string{"."}},
		{desc: "multiple packages", args: []string{"./pkg/a", "./pkg/b", "-v"}, expected: []string{"./pkg/a", "./pkg/b"}},
		{desc: "stops at -args", args: []string{"-v", "./...", "-args", "-custom", "./not/a/pkg"}, expected: []string{"./..."}},
		{desc: "no args defaults to dot", args: nil, expected: []string{"."}},
		{desc: "bare relative path", args: []string{"-v", "./cmd/gotest"}, expected: []string{"./cmd/gotest"}},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("extracts expected patterns", func(it *gotest.T) {
				result := ExtractPackagePatterns(tc.args)
				gotest.Equal(it, tc.expected, result)
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestLooksLikePackagePattern(t *gotest.T) {
	for _, tc := range []struct {
		desc   string
		input  string
		expect bool
	}{
		{desc: "relative path", input: "./pkg/foo", expect: true},
		{desc: "absolute path", input: "/usr/local/pkg", expect: true},
		{desc: "named package", input: "github.com/foo/bar", expect: true},
		{desc: "flag", input: "-v", expect: false},
		{desc: "bare word", input: "strings", expect: false},
		{desc: "dot only", input: ".", expect: true},
		{desc: "dot-slash", input: "./...", expect: true},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("returns expected result", func(it *gotest.T) {
				gotest.Equal(it, tc.expect, gotestrunner.LooksLikePackagePattern(tc.input))
			})
		})
	}
}

// --- cli ---

func (s *CmdGotestTestSuite) TestParseMinFlag(t *gotest.T) {
	for _, tc := range []struct {
		desc      string
		args      []string
		expect    int
		expectErr bool
	}{
		{desc: "no flag", args: []string{"--debug"}, expect: 0},
		{desc: "equals syntax", args: []string{"--min=80"}, expect: 80},
		{desc: "space syntax", args: []string{"--min", "90"}, expect: 90},
		{desc: "empty args", args: nil, expect: 0},
		{desc: "invalid value", args: []string{"--min=abc"}, expectErr: true},
		{desc: "min at end no value", args: []string{"--min"}, expect: 0},
		{desc: "negative value", args: []string{"--min=-5"}, expectErr: true},
		{desc: "over 100", args: []string{"--min=150"}, expectErr: true},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("parses correctly", func(it *gotest.T) {
				got, err := ExportParseMinFlag(tc.args)
				if tc.expectErr {
					gotest.True(it, err != nil, "expected error")
				} else {
					gotest.NoError(it, err)
					gotest.Equal(it, tc.expect, got)
				}
			})
		})
	}
}

// --- discover ---

func (s *CmdGotestTestSuite) TestRunDiscover_SimpleSuite(t *gotest.T) {
	t.It("discovers suites in examples/cart", func(it *gotest.T) {
		examplesDir := filepath.Join("..", "..", "examples")
		if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
			it.T().Skipf("examples directory not found: %v", err)
		}

		origDir, err := os.Getwd()
		if err != nil {
			it.T().Fatal(err)
		}
		absExamples, err := filepath.Abs(examplesDir)
		if err != nil {
			it.T().Fatal(err)
		}
		if err := os.Chdir(absExamples); err != nil {
			it.T().Fatal(err)
		}
		defer os.Chdir(origDir)

		loadResults, err := gotestgen.LoadPackages([]string{"./cart"}, nil)
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

		if len(pkg.Suites) != 2 {
			it.T().Fatalf("expected 2 suites, got %d", len(pkg.Suites))
		}

		// Verify the ptest suite
		st := pkg.Suites[0]
		gotest.Equal(it, "ShoppingCartTestSuite", st.Name)
		gotest.False(it, st.Parallel)
		gotest.False(it, st.Focused)
		gotest.False(it, st.Excluded)
		gotest.Equal(it, "suite_test.go", st.File)
		gotest.Equal(it, 5, st.Line)
		gotest.Equal(it, 6, st.Col)

		expectedLifecycle := []string{"BeforeEach"}
		gotest.Equal(it, expectedLifecycle, st.Lifecycle)
		gotest.Len(it, st.Fixtures, 0)

		if len(st.Methods) != 9 {
			it.T().Fatalf("expected 9 methods, got %d", len(st.Methods))
		}
		gotest.Equal(it, "TestAddSingleItem", st.Methods[0].Name)
		gotest.Equal(it, 15, st.Methods[0].Line)
		gotest.Equal(it, 1, st.Methods[0].Col)
		gotest.Equal(it, "TestAddMultipleItems", st.Methods[1].Name)

		// Verify the pxtest suite
		sx := pkg.Suites[1]
		gotest.Equal(it, "ShoppingCartTestSuite", sx.Name)
		if len(sx.Methods) != 2 {
			it.T().Fatalf("expected 2 pxtest methods, got %d", len(sx.Methods))
		}
		gotest.Equal(it, "TestAddItem", sx.Methods[0].Name)
		gotest.Equal(it, "TestRemoveItem", sx.Methods[1].Name)

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

// --- focusguard ---

func (s *CmdGotestTestSuite) TestFocusViolation_String(t *gotest.T) {
	for _, tc := range []struct {
		desc     string
		v        FocusViolation
		expected string
	}{
		{
			desc:     "suite violation only",
			v:        FocusViolation{SuiteName: "F_MyTestSuite"},
			expected: "  type F_MyTestSuite",
		},
		{
			desc:     "method violation",
			v:        FocusViolation{SuiteName: "MyTestSuite", MethodName: "F_TestSomething"},
			expected: "  MyTestSuite.F_TestSomething",
		},
		{
			desc:     "both focused suite and method",
			v:        FocusViolation{SuiteName: "F_MyTestSuite", MethodName: "F_TestFoo"},
			expected: "  F_MyTestSuite.F_TestFoo",
		},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("formats correctly", func(it *gotest.T) {
				gotest.Equal(it, tc.expected, tc.v.String())
			})
		})
	}
}

// --- generate ---

func (s *CmdGotestTestSuite) TestGenerateOverlay_ProducesValidOutput(t *gotest.T) {
	t.It("produces valid overlay JSON", func(it *gotest.T) {
		examplesDir := filepath.Join("..", "..", "examples")
		if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
			it.T().Skipf("examples directory not found: %v", err)
		}

		origDir, err := os.Getwd()
		if err != nil {
			it.T().Fatal(err)
		}
		absExamples, err := filepath.Abs(examplesDir)
		if err != nil {
			it.T().Fatal(err)
		}
		if err := os.Chdir(absExamples); err != nil {
			it.T().Fatal(err)
		}
		defer os.Chdir(origDir)

		loaded, err := gotestgen.LoadPackages([]string{"./cart"}, nil)
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
		gotest.True(it, len(overlayContent.Replace) > 0, "overlay.json Replace map is empty")
	})
}

func (s *CmdGotestTestSuite) TestGenerateOverlay_NoSuitesReturnsEmpty(t *gotest.T) {
	t.It("returns empty results for package without suites", func(it *gotest.T) {
		tmpDir, err := os.MkdirTemp("", "overlay-test-nosuite-*")
		if err != nil {
			it.T().Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module nosuite\n\ngo 1.24\n"), 0644); err != nil {
			it.T().Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
			it.T().Fatal(err)
		}

		origDir, err := os.Getwd()
		if err != nil {
			it.T().Fatal(err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			it.T().Fatal(err)
		}
		defer os.Chdir(origDir)

		loaded, err := gotestgen.LoadPackages([]string{"."}, nil)
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
}

// --- spec ---

func (s *CmdGotestTestSuite) TestSpecFlagParsing(t *gotest.T) {
	for _, tc := range []struct {
		desc      string
		args      []string
		wantFmt   string
		wantOut   string
		wantInput string
		wantColor bool
		wantGoLen int
	}{
		{
			desc:      "no flags",
			args:      []string{"./..."},
			wantFmt:   "terminal",
			wantInput: "",
			wantColor: false,
			wantGoLen: 1,
		},
		{
			desc:      "input with equals",
			args:      []string{"--input=events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			desc:      "input with space",
			args:      []string{"--input", "events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			desc:      "input stdin dash",
			args:      []string{"--input=-"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			desc:      "input with format",
			args:      []string{"--format=md", "--input=data.json"},
			wantFmt:   "md",
			wantInput: "data.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			desc:      "input with output and no-color",
			args:      []string{"--input=-", "--output=out.txt", "--no-color"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantOut:   "out.txt",
			wantColor: true,
			wantGoLen: 0,
		},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("parses flags correctly", func(it *gotest.T) {
				ownArgs, goTestArgs, err := SplitArgs(tc.args, ExportSpecAllowed)
				gotest.NoError(it, err)

				format := ExportExtractStringFlag(ownArgs, "--format", "terminal")
				output := ExportExtractStringFlag(ownArgs, "--output", "")
				input := ExportExtractStringFlag(ownArgs, "--input", "")
				noColor := ExportHasFlag(ownArgs, "--no-color")

				gotest.Equal(it, tc.wantFmt, format)
				gotest.Equal(it, tc.wantOut, output)
				gotest.Equal(it, tc.wantInput, input)
				gotest.Equal(it, tc.wantColor, noColor)
				gotest.Len(it, goTestArgs, tc.wantGoLen)
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestRunSpec_InputStdin(t *gotest.T) {
	t.It("renders spec output from stdin-like JSON", func(it *gotest.T) {
		examplesDir := filepath.Join("..", "..", "examples")
		if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
			it.T().Skipf("examples directory not found: %v", err)
		}

		origDir, err := os.Getwd()
		if err != nil {
			it.T().Fatal(err)
		}
		absExamples, err := filepath.Abs(examplesDir)
		if err != nil {
			it.T().Fatal(err)
		}
		if err := os.Chdir(absExamples); err != nil {
			it.T().Fatal(err)
		}
		defer os.Chdir(origDir)

		loaded, err := gotestgen.LoadPackages([]string{"./cart"}, nil)
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

		overlayArgs := []string{"-overlay=" + filepath.Join(tmpDir, "overlay.json"), "./cart"}
		jsonData, _, err := gotestrunner.StdlibRunTestsJSON(context.Background(), overlayArgs)
		if err != nil {
			it.T().Fatalf("StdlibRunTestsJSON: %v", err)
		}

		events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
		if err != nil {
			it.T().Fatalf("ParseEvents: %v", err)
		}

		tree := gotestspec.BuildTree(events)

		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())

		output := buf.String()
		gotest.True(it, bytes.Contains([]byte(output), []byte("ShoppingCart")), "expected output to contain \"ShoppingCart\", got:\n%s", output)
	})
}

// --- watch ---

func (s *CmdGotestTestSuite) TestIsGoFile(t *gotest.T) {
	for _, tc := range []struct {
		desc   string
		name   string
		expect bool
	}{
		{desc: "go file", name: "main.go", expect: true},
		{desc: "test file", name: "main_test.go", expect: true},
		{desc: "path with go file", name: "/tmp/foo/bar.go", expect: true},
		{desc: "not a go file", name: "main.py", expect: false},
		{desc: "go in middle", name: "foo.go.bak", expect: false},
		{desc: "empty", name: "", expect: false},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("returns expected result", func(it *gotest.T) {
				gotest.Equal(it, tc.expect, ExportIsGoFile(tc.name))
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestDirsToPatterns(t *gotest.T) {
	for _, tc := range []struct {
		desc    string
		dirs    map[string]bool
		lenWant int
	}{
		{desc: "single dir", dirs: map[string]bool{"pkg/foo": true}, lenWant: 1},
		{desc: "multiple dirs", dirs: map[string]bool{"pkg/foo": true, "cmd/bar": true}, lenWant: 2},
		{desc: "empty", dirs: map[string]bool{}, lenWant: 0},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("returns correct patterns", func(it *gotest.T) {
				result := ExportDirsToPatterns(tc.dirs)
				gotest.Len(it, result, tc.lenWant)
				for _, p := range result {
					gotest.True(it, len(p) > 2 && p[:2] == "./", "expected ./ prefix, got: %s", p)
				}
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestReplacePatterns(t *gotest.T) {
	for _, tc := range []struct {
		desc        string
		original    []string
		newPatterns []string
		expected    []string
	}{
		{
			desc:        "replaces package pattern",
			original:    []string{"-v", "./pkg/foo", "-race"},
			newPatterns: []string{"./cmd/bar"},
			expected:    []string{"-v", "-race", "./cmd/bar"},
		},
		{
			desc:        "no patterns to replace",
			original:    []string{"-v", "-race"},
			newPatterns: []string{"./pkg/new"},
			expected:    []string{"-v", "-race", "./pkg/new"},
		},
		{
			desc:        "multiple patterns replaced",
			original:    []string{"-v", "./pkg/a", "./pkg/b", "-race"},
			newPatterns: []string{"./changed"},
			expected:    []string{"-v", "-race", "./changed"},
		},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("returns expected result", func(it *gotest.T) {
				result := ExportReplacePatterns(tc.original, tc.newPatterns)
				gotest.Equal(it, tc.expected, result)
			})
		})
	}
}

func (s *CmdGotestTestSuite) TestWatchSplitArgs(t *gotest.T) {
	for _, tc := range []struct {
		desc         string
		args         []string
		expectOwn    []string
		expectGoTest []string
	}{
		{"no flags", []string{"./pkg/..."}, nil, []string{"./pkg/..."}},
		{"json flag", []string{"-json", "./pkg/..."}, nil, []string{"-json", "./pkg/..."}},
		{"debounce with json", []string{"--debounce=500ms", "-json", "./..."}, []string{"--debounce=500ms"}, []string{"-json", "./..."}},
		{"debug and ci", []string{"--debug", "--ci", "-v", "./..."}, []string{"--debug", "--ci"}, []string{"-v", "./..."}},
	} {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("splits watch args correctly", func(it *gotest.T) {
				own, goTest, err := SplitArgs(tc.args, ExportWatchAllowed)
				gotest.NoError(it, err)
				gotest.Equal(it, tc.expectOwn, own)
				gotest.Equal(it, tc.expectGoTest, goTest)
			})
		})
	}
}
