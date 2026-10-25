package main_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// PatternArgsTestSuite runs the built CLI on positional arguments go test
// reads its own way: an empty pattern, and Go files.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type PatternArgsTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *PatternArgsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *PatternArgsTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// notestfilesDir is a package without test files: every mode matches it and
// verifies nothing, which passes.
func (s *PatternArgsTestSuite) notestfilesDir() string {
	return filepath.Join(s.cli.repoRoot, "cmd", "gotest", "testdata", "unrun", "notestfiles")
}

// TestAnEmptyPatternIsTheCurrentDirectory: scripts pass "$PKG" unset, and
// go test reads "" as ".".
func (s *PatternArgsTestSuite) TestAnEmptyPatternIsTheCurrentDirectory(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"a text run", []string{""}},
		{"a -json run", []string{"-json", ""}},
		{"beside another pattern", []string{"", "./"}},
		{"spec", []string{"spec", "--no-color", ""}},
		{"summary", []string{"summary", "--no-color", ""}},
		{"bench", []string{"bench", ""}},
		{"fuzz", []string{"fuzz", ""}},
		{"generate", []string{"generate", ""}},
		{"clean", []string{"clean", ""}},
		{"discover", []string{"discover", ""}},
		{"lint", []string{"lint", ""}},
		{"lint through the go/analysis driver", []string{"lint", "-debug=", ""}},
	}) {
		_, stderr, code := s.cli.runSplitIn(t, s.notestfilesDir(), tc.args...)
		gotest.Equal(t, 0, code, "stderr:\n%s", stderr)
		gotest.NotContains(t, stderr, "matched no packages")
	}
}

// TestABadFlagValueIsAUsageError: the test binary would fail on it after the
// build, which reads as a failing test.
func (s *PatternArgsTestSuite) TestABadFlagValueIsAUsageError(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"a text run", []string{"-count=abc"}},
		{"spec", []string{"spec", "-count=abc"}},
		{"bench", []string{"bench", "-count=abc"}},
	}) {
		_, stderr, code := s.cli.runSplitIn(t, s.notestfilesDir(), tc.args...)
		gotest.Equal(t, 2, code, "stderr:\n%s", stderr)
		gotest.Contains(t, stderr, `invalid value "abc" for flag -count`)
	}
}

// TestGoFilesAreAUsageError: go test builds files as an ad-hoc package, which
// has no suites; gotest names them instead of failing that package's build.
func (s *PatternArgsTestSuite) TestGoFilesAreAUsageError(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"a text run", []string{"notestfiles.go"}},
		{"a -json run", []string{"-json", "notestfiles.go"}},
		{"spec", []string{"spec", "notestfiles.go"}},
		{"summary", []string{"summary", "notestfiles.go"}},
		{"bench", []string{"bench", "notestfiles.go"}},
		{"fuzz", []string{"fuzz", "notestfiles.go"}},
		{"generate", []string{"generate", "notestfiles.go"}},
		{"clean", []string{"clean", "notestfiles.go"}},
		{"discover", []string{"discover", "notestfiles.go"}},
	}) {
		stdout, stderr, code := s.cli.runSplitIn(t, s.notestfilesDir(), tc.args...)
		gotest.Equal(t, 2, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		gotest.Equal(t, "FAIL: gotest takes packages, not files: notestfiles.go\n", stderr)
		gotest.Empty(t, stdout)
	}
}
