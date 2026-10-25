//go:build unix

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OrphansTestSuite: a run leaves no process of its suites behind.
type OrphansTestSuite struct {
	pid int // the sleep the run's test started, ended should it outlive the check
}

func (s *OrphansTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = time.Minute // 4s locally, cold under -race on 4 cores
	return cfg
}

func (s *OrphansTestSuite) AfterEach(t *gotest.T) {
	if s.pid > 0 && sleeping(s.pid) {
		_ = syscall.Kill(s.pid, syscall.SIGKILL)
	}
}

func (s *OrphansTestSuite) TestSleepLeftBehind(t *gotest.T) {
	dir := t.TempDir()
	run, err := testkit.RunPackage(t.Context(), dir, "../testdata/orphan/")
	gotest.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "orphan-pid"))
	gotest.NoError(t, err, "the test recorded no pid; stdout: %s", run.Stdout)
	s.pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
	gotest.NoError(t, err)
	t.When("a test starts sleep 4242 and never waits for it", func(w *gotest.T) {
		w.It("the test passes and no such process outlives the run", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestOrphanTestSuite/TestLeavesAProcess"), "stdout: %s", run.Stdout)
			gotest.Eventually(it, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
				gotest.False(poll, sleeping(s.pid), "sleep 4242 outlived the run")
			})
		})
	})
}

// sleeping reports whether pid is still the orphan suite's sleep 4242, not
// a zombie and not a process that reused the pid.
func sleeping(pid int) bool {
	out, _ := exec.Command("ps", "-o", "stat=,args=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // G204: a fixed command, a numeric pid
	stat, args, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	return stat != "" && !strings.HasPrefix(stat, "Z") && strings.TrimSpace(args) == "sleep 4242"
}
