package main_test

import (
	"encoding/json"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// VerdictShapeTestSuite runs the built CLI and holds its verdicts to the
// shape go test gives them, so every stream consumer reads them alike.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type VerdictShapeTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *VerdictShapeTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *VerdictShapeTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const (
	shapeRepo  = "github.com/mvrahden/go-test/cmd/gotest/testdata/"
	plainDir   = "./cmd/gotest/testdata/census/plain/"
	benchDir   = "./cmd/gotest/testdata/census/benchplain/"
	noFilesDir = "./cmd/gotest/testdata/unrun/notestfiles/"
	stdlibDir  = "./cmd/gotest/testdata/unrun/nosuites/"
	xonlyDir   = "./cmd/gotest/testdata/unrun/excluded/"
)

func (s *VerdictShapeTestSuite) TestTextVerdictsFollowPackageOrder(t *gotest.T) {
	stdout, stderr, code := s.cli.runSplit(t, noFilesDir, plainDir, stdlibDir)
	gotest.Equal(t, 0, code, stderr)
	gotest.Regexp(t, `^\?   \t`+shapeRepo+`unrun/notestfiles\t\[no test files\]\nok  \t`+shapeRepo+`census/plain\t\d+\.\d{3}s\n\?   \t`+shapeRepo+`unrun/nosuites\t\[no suites\]\n$`, stdout)
}

func (s *VerdictShapeTestSuite) TestStreamOpensEachPackageOnce(t *gotest.T) {
	stream, stderr, code := s.cli.runSplit(t, "-json", plainDir, noFilesDir, xonlyDir, "errors")
	gotest.Equal(t, 0, code, stderr)

	first := map[string]string{}
	starts := map[string]int{}
	for line := range strings.Lines(stream) {
		var ev struct{ Action, Package string }
		gotest.NoError(t, json.Unmarshal([]byte(line), &ev), line)
		if ev.Package == "" {
			continue
		}
		if _, ok := first[ev.Package]; !ok {
			first[ev.Package] = ev.Action
		}
		if ev.Action == "start" {
			starts[ev.Package]++
		}
	}
	for _, pkg := range []string{shapeRepo + "census/plain", shapeRepo + "unrun/notestfiles", shapeRepo + "unrun/excluded", "errors"} {
		t.It("opens "+pkg+" with one start before its other events", func(it *gotest.T) {
			gotest.Equal(it, "start", first[pkg])
			gotest.Equal(it, 1, starts[pkg])
		})
	}
}

func (s *VerdictShapeTestSuite) TestRunThatDeselectsEveryMethod(t *gotest.T) {
	stdout, stderr, code := s.cli.runSplit(t, plainDir, "-run", "PlainTestSuite/NoSuchMethod")
	gotest.Equal(t, 0, code, stderr)
	gotest.Regexp(t, `^ok  \t`+shapeRepo+`census/plain\t\d+\.\d{3}s \[no tests to run\]\n$`, stdout)
}

func (s *VerdictShapeTestSuite) TestBenchGivesEveryPackageAVerdict(t *gotest.T) {
	stdout, stderr, code := s.cli.runSplit(t, "bench", "-benchtime=1x", benchDir, plainDir, xonlyDir)
	gotest.Equal(t, 0, code, stderr)

	t.It("passes a package with suites but no benchmark as go test -bench does", func(it *gotest.T) {
		gotest.Contains(it, stdout, "ok  \t"+shapeRepo+"census/plain\t0.000s\n")
	})
	t.It("passes a package whose suites are all excluded the same way", func(it *gotest.T) {
		gotest.Contains(it, stdout, "ok  \t"+shapeRepo+"unrun/excluded\t0.000s\n")
	})
	t.It("still runs the benchmarks", func(it *gotest.T) {
		gotest.Regexp(it, `ok  \t`+shapeRepo+`census/benchplain\t\d+\.\d{3}s\n`, stdout)
	})
}
