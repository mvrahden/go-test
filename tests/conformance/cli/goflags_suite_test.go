package cli_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIGoTestFlagsTestSuite: the go test flags gotest takes over keep the
// meaning go test gives them, or are refused with a reason.
type CLIGoTestFlagsTestSuite struct{}

func (s *CLIGoTestFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLIGoTestFlagsTestSuite) TestOwnedGoTestFlags(t *gotest.T) {
	t.When("a go test flag gotest implements itself is given", func(w *gotest.T) {
		w.It("keeps the meaning go test gives the flag, or refuses it with a reason", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "-run naming a fuzz wrapper selects its suite and replays the seeds", args: []string{"-v", "-run", "FuzzFuzzyTestSuite_FuzzUpperIsIdempotent", "../testdata/fuzzing/"}, exit: 0, stdoutHas: "--- PASS: FuzzFuzzyTestSuite_FuzzUpperIsIdempotent"},
				{Desc: "-exec wraps every suite binary", args: []string{"-exec", "env", "../testdata/passing/"}, exit: 0, stdoutHas: "ok"},
				{Desc: "-p caps the processes and the run still completes", args: []string{"-p", "1", "../testdata/passing/", "../testdata/covered/"}, exit: 0},
				{Desc: "-o is refused with a reason", args: []string{"-o", "out.test", "../testdata/passing/"}, exit: 2, stderrHas: "-o"},
				{Desc: "-buildvcs=false reaches the build", args: []string{"-buildvcs=false", "../testdata/passing/"}, exit: 0, stdoutHas: "ok"},
				{Desc: "-pgo=off reaches the build", args: []string{"-pgo=off", "../testdata/passing/"}, exit: 0, stdoutHas: "ok"},
				{Desc: "the last -v wins", args: []string{"-v", "-v=false", "../testdata/passing/"}, exit: 0, stdoutHas: "ok", stdoutNot: "PASS"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				if row.stdoutHas != "" {
					gotest.Contains(sub, run.Stdout, row.stdoutHas)
				}
				if row.stdoutNot != "" {
					gotest.NotContains(sub, run.Stdout, row.stdoutNot)
				}
				if row.stderrHas != "" {
					gotest.Contains(sub, run.Stderr, row.stderrHas)
				}
			}
		})
	})
}

func (s *CLIGoTestFlagsTestSuite) TestAUserOverlay(t *gotest.T) {
	empty := filepath.Join(t.TempDir(), "o.json")
	gotest.NoError(t, os.WriteFile(empty, []byte(`{"Replace":{}}`), 0o600))
	refused, err := testkit.RunCLI(t.Context(), ".", "-overlay", empty, "../testdata/failing/")
	gotest.NoError(t, err)
	viaEnv, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOFLAGS=-overlay=" + empty}, "../testdata/failing/")
	gotest.NoError(t, err)
	t.When("-overlay is given to a run", func(w *gotest.T) {
		w.It("refuses it with exit 2 before anything is built, saying gotest compiles its harness through an overlay of its own", func(it *gotest.T) {
			gotest.Equal(it, 2, refused.Exit, "stdout: %s\nstderr: %s", refused.Stdout, refused.Stderr)
			gotest.Contains(it, refused.Stderr, "FAIL: -overlay is not supported: gotest compiles its generated harness through an overlay of its own")
			gotest.Empty(it, refused.Stdout)
		})
		w.It("still compiles its harness when GOFLAGS carries an -overlay", func(it *gotest.T) {
			gotest.Equal(it, 1, viaEnv.Exit, "stdout: %s\nstderr: %s", viaEnv.Stdout, viaEnv.Stderr)
		})
	})
}

func (s *CLIGoTestFlagsTestSuite) TestABuildOnlyModeOrTheFuzzEngine(t *gotest.T) {
	t.When("-c, -n or -fuzz is given to a run", func(w *gotest.T) {
		w.It("refuses each with exit 2, naming what does that job instead", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "-c is refused with what builds a binary", args: []string{"-c", "../testdata/passing/"}, exit: 2, stderrHas: "-c is not supported"},
				{Desc: "-n is refused with what keeps the work dir", args: []string{"-n", "../testdata/passing/"}, exit: 2, stderrHas: "-n is not supported"},
				{Desc: "-fuzz is refused with the command that fuzzes", args: []string{"-fuzz=FuzzX", "../testdata/passing/"}, exit: 2, stderrHas: "-fuzz is not a run flag"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, row.stderrHas)
			}
		})
	})
}

func (s *CLIGoTestFlagsTestSuite) TestAProfileFlagOverSeveralSuiteProcesses(t *gotest.T) {
	dir := t.TempDir()
	two, err := testkit.RunCLI(t.Context(), ".", "-cpuprofile", filepath.Join(dir, "two.out"), "../testdata/passing/", "../testdata/covered/")
	gotest.NoError(t, err)
	one, err := testkit.RunCLI(t.Context(), ".", "-cpuprofile", filepath.Join(dir, "one.out"), "../testdata/passing/")
	gotest.NoError(t, err)
	t.When("-cpuprofile, -memprofile, -blockprofile, -mutexprofile or -trace is given to a run that starts two suite processes", func(w *gotest.T) {
		w.It("refuses it with exit 2, naming the flag and how many processes would write its file", func(it *gotest.T) {
			gotest.Equal(it, 2, two.Exit, "stdout: %s\nstderr: %s", two.Stdout, two.Stderr)
			gotest.Contains(it, two.Stderr, "FAIL: -cpuprofile writes one file, and this run starts 2 suite processes that would each write it: select one suite of one package with -run '^Test<Suite>$'")
		})
		w.It("a run of one suite writes the profile", func(it *gotest.T) {
			gotest.Equal(it, 0, one.Exit, "stdout: %s\nstderr: %s", one.Stdout, one.Stderr)
			_, statErr := os.Stat(filepath.Join(dir, "one.out"))
			gotest.NoError(it, statErr)
		})
	})
}
