package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoppingTestSuite stops a run at the moments a stop used to leave fixtures
// up: while they come up, while a test holds what a teardown needs.
type StoppingTestSuite struct {
	bin map[string]string // per testdata/stopping package
}

func (s *StoppingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *StoppingTestSuite) BeforeAll(t *gotest.T) {
	sigintForChildren()
	root, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	cli, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	s.bin = map[string]string{}
	for _, pkg := range []string{"setup", "hold", "quick"} {
		dir := "./testdata/stopping/" + pkg + "/"
		out, err := exec.Command(cli, "generate", dir).CombinedOutput() //nolint:gosec // G204: controlled binary with fixed args
		gotest.NoError(t, err, string(out))
		name := pkg + ".test"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		s.bin[pkg] = filepath.Join(t.TempDir(), name)
		out, err = exec.Command("go", "test", "-c", "-o", s.bin[pkg], dir).CombinedOutput() //nolint:gosec // G204: fixed args
		gotest.NoError(t, err, string(out))
	}
}

func (s *StoppingTestSuite) AfterAll(t *gotest.T) {
	generated, _ := filepath.Glob("testdata/stopping/*/gotest_p*suite_test.go")
	for _, f := range generated {
		_ = os.Remove(f)
	}
}

// announced starts the binary as the runner does, with a stop file, and
// returns the file to create before interrupting.
func (s *StoppingTestSuite) announced(t *gotest.T, pkg, run string, env ...string) (*exec.Cmd, string) {
	stop := filepath.Join(t.TempDir(), "stop")
	cmd := exec.Command(s.bin[pkg], "-test.run="+run) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(append(ownEnv(), protocol.EnvStopFile+"="+stop), env...)
	return cmd, stop
}

// A stop while a fixture is still coming up cancels it; what already came up
// is torn down, and nothing is left behind.
func (s *StoppingTestSuite) TestAStopDuringSetupReleasesWhatCameUp(t *gotest.T) {
	cmd, stop := s.announced(t, "setup", "TestSlowSetupTestSuite")
	tree, log := startTreeUntil(t, cmd, "STOPPING_LOG", "container starting")

	gotest.NoError(t, os.WriteFile(stop, nil, 0o600))
	gotest.NoError(t, tree.Interrupt())
	err := wait(tree, cmd)

	gotest.Equal(t, []string{"db up", "container starting", "db down"}, readEvents(log))
	gotest.True(t, stoppedFromOutside(err), "the process still ends as stopped from outside: %v", err)
}

// A test holding what a fixture's teardown waits for is told to stop first —
// its context ends — so the teardown gets it back instead of waiting it out.
func (s *StoppingTestSuite) TestAStopStopsTheTestsFirst(t *gotest.T) {
	cmd, stop := s.announced(t, "hold", "TestHoldingTestSuite", "STOPPING_HOLD=1")
	tree, log := startTreeUntil(t, cmd, "STOPPING_LOG", "running")

	began := time.Now()
	gotest.NoError(t, os.WriteFile(stop, nil, 0o600))
	gotest.NoError(t, tree.Interrupt())
	err := wait(tree, cmd)

	gotest.Equal(t, []string{"db up", "running", "pool down", "db down"}, readEvents(log))
	gotest.Less(t, time.Since(began), 10*time.Second, "the teardown waited for the test")
	gotest.True(t, stoppedFromOutside(err), "%v", err)
}

// The runner's sidecar variables are read once and taken out of the
// environment: a process a test starts must not answer to them.
func (s *StoppingTestSuite) TestTheRunnersFilesStayWithTheRunner(t *gotest.T) {
	cmd, _ := s.announced(t, "quick", "TestEnvTestSuite", "STOPPING_ENV=1")
	log := filepath.Join(t.TempDir(), "events")
	cmd.Env = append(cmd.Env, "STOPPING_LOG="+log)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, string(out))
	gotest.Equal(t, []string{"env="}, readEvents(log))
}
