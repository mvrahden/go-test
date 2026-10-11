package main_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// NoPackagesTestSuite runs the built CLI on patterns that match no package:
// a run that verified nothing is not a pass, in any mode.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type NoPackagesTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *NoPackagesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *NoPackagesTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// nogo is a directory without Go files: go test reports it "matched no packages".
const nogo = "./cmd/gotest/testdata/unrun/nogo/..."

func (s *NoPackagesTestSuite) TestEveryModeExits2(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"a text run", []string{nogo}},
		{"a -json run", []string{"-json", nogo}},
		{"spec", []string{"spec", "--no-color", nogo}},
		{"spec --static", []string{"spec", "--static", "--no-color", nogo}},
		{"summary", []string{"summary", "--no-color", nogo}},
		{"bench", []string{"bench", nogo}},
		{"fuzz", []string{"fuzz", nogo}},
	}) {
		stdout, stderr, code := s.cli.runSplit(t, tc.args...)

		t.It("exits 2", func(it *gotest.T) {
			gotest.Equal(it, 2, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		})
		t.It("names the pattern on stderr", func(it *gotest.T) {
			gotest.Equal(it, "FAIL: \""+nogo+"\" matched no packages\n", stderr)
		})
		t.It("writes nothing on stdout", func(it *gotest.T) {
			gotest.Empty(it, stdout)
		})
	}
}

func (s *NoPackagesTestSuite) TestModuleWithoutPackages(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/empty\n\ngo 1.26\n"), 0o600))

	stdout, stderr, code := s.cli.runSplitIn(t, dir, "./...")
	gotest.Equal(t, 2, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
	gotest.Equal(t, "FAIL: \"./...\" matched no packages\n", stderr)
	gotest.Empty(t, stdout)
}

func (s *NoPackagesTestSuite) TestAPatternBesideOthersThatMatched(t *gotest.T) {
	const notestfiles = "./cmd/gotest/testdata/unrun/notestfiles/"
	const verdict = "?   \tgithub.com/mvrahden/go-test/cmd/gotest/testdata/unrun/notestfiles\t[no test files]\n"
	const warning = "warning: \"" + nogo + "\" matched no packages\n"

	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"a text run", []string{notestfiles, nogo}},
		{"a -json run", []string{"-json", notestfiles, nogo}},
	}) {
		stdout, stderr, code := s.cli.runSplit(t, tc.args...)

		t.It("runs the rest and passes", func(it *gotest.T) {
			gotest.Equal(it, 0, code, "stderr:\n%s", stderr)
			gotest.Contains(it, stdout, "[no test files]")
		})
		t.It("warns about the pattern on stderr only", func(it *gotest.T) {
			gotest.Equal(it, warning, stderr)
			gotest.NotContains(it, stdout, "matched no packages")
		})
	}

	t.It("prints the package verdict as go test does", func(it *gotest.T) {
		stdout, _, _ := s.cli.runSplit(it, notestfiles, nogo)
		gotest.Equal(it, verdict, stdout)
	})
}

// TestWatchKeepsWatching starts watch on a pattern that matches nothing yet:
// the iteration fails, and watch stays up for the package about to be written.
func (s *NoPackagesTestSuite) TestWatchKeepsWatching(t *gotest.T) {
	cmd := exec.Command(s.cli.binary, "watch", "--", "-json", nogo) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.cli.repoRoot
	stdout, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	gotest.NoError(t, cmd.Start())

	line, err := bufio.NewReader(stdout).ReadString('\n')
	done := make(chan struct{})
	wait := gotest.Go(t, func() {
		_ = cmd.Wait()
		close(done)
	})
	exited := false
	select {
	case <-done:
		exited = true
	case <-time.After(2 * time.Second):
	}
	_ = cmd.Process.Kill()
	wait()

	gotest.NoError(t, err)
	gotest.Equal(t, `{"Action":"watch-error","Output":"\"`+nogo+`\" matched no packages"}`+"\n", line)
	gotest.False(t, exited, "watch exited instead of watching")
}

// TestANonexistentDirectory names a path that does not exist: it fails to
// load, a build failure with go test's own diagnostic.
func (s *NoPackagesTestSuite) TestANonexistentDirectory(t *gotest.T) {
	const missing = "./cmd/gotest/testdata/unrun/nosuchdir/..."
	stdout, stderr, code := s.cli.runSplit(t, missing)

	t.It("exits 2 with a failed package", func(it *gotest.T) {
		gotest.Equal(it, 2, code, "stderr:\n%s", stderr)
		gotest.Equal(it, "FAIL\t"+missing+" [build failed]\nFAIL\n", stdout)
	})
	t.It("prints the diagnostic as go test does, without a position placeholder", func(it *gotest.T) {
		gotest.Regexp(it, `^# `+regexp.QuoteMeta(missing)+`\npattern `+regexp.QuoteMeta(missing)+`: `, stderr)
	})
}
