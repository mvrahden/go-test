package main_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// SkipFlagTestSuite runs the built CLI with go test's -skip, which reaches
// the suite binaries as -run does.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type SkipFlagTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *SkipFlagTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SkipFlagTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const skipFixture = `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type SkipTestSuite struct{}

func (s *SkipTestSuite) TestKept(t *gotest.T) {}

func (s *SkipTestSuite) TestBroken(t *gotest.T) { gotest.Fail(t, "skipped method ran") }

func (s *SkipTestSuite) BenchmarkKept(b *gotest.B) { for b.Loop() {} }

func (s *SkipTestSuite) BenchmarkSkipped(b *gotest.B) { for b.Loop() {} }
`

func (s *SkipFlagTestSuite) TestSkipsTheMatchingMethod(t *gotest.T) {
	dir := stageFixtureModule(t, s.cli.repoRoot, t.TempDir(), "skip_test.go", []byte(skipFixture))

	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"a text run", []string{"-v", "-skip", "TestSkipTestSuite/TestBroken", "."}},
		{"a -json run", []string{"-json", "-skip=TestSkipTestSuite/TestBroken", "."}},
		{"spec", []string{"spec", "--no-color", "-skip", "TestSkipTestSuite/TestBroken", "."}},
	}) {
		stdout, stderr, code := s.cli.runSplitIn(t, dir, tc.args...)
		gotest.Equal(t, 0, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		gotest.NotContains(t, stdout+stderr, "skipped method ran")
		gotest.Contains(t, stdout, "Kept")
	}

	t.It("skips a benchmark in bench", func(it *gotest.T) {
		stdout, stderr, code := s.cli.runSplitIn(it, dir, "bench", "-benchtime=1x", "-skip", "BenchmarkSkipTestSuite/BenchmarkSkipped", ".")
		gotest.Equal(it, 0, code, "stderr:\n%s", stderr)
		gotest.Contains(it, stdout, "BenchmarkSkipTestSuite/BenchmarkKept")
		gotest.NotContains(it, stdout, "BenchmarkSkipped")
	})
}
