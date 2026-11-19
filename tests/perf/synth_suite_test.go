package perf_test

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SynthTestSuite holds the harness the other perf suites read: the staged
// module runs, and the shim and the edit do what they claim.
type SynthTestSuite struct {
	st staging
}

// Exclusive, as every perf suite: the CPU suite measures with nothing else
// of the run beside it.
func (s *SynthTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 2 * time.Minute, SetupTimeout: 3 * time.Minute}
}

func (s *SynthTestSuite) BeforeAll(t *gotest.T) { s.st.setUp(t) }

func (s *SynthTestSuite) AfterAll(t *gotest.T) { s.st.tearDown(t) }

func (s *SynthTestSuite) TestTheStagedModuleRuns(t *gotest.T) {
	m, err := s.st.h.measure(t.Context(), s.st.m12, s.st.cli, s.st.env, "./g/...")
	gotest.NoError(t, err)
	t.It("passes under gotest", func(it *gotest.T) {
		gotest.Equal(it, 0, m.Exit, "stderr: %s", m.Stderr)
		gotest.Len(it, regexp.MustCompile(`(?m)^ok\s`).FindAll(m.Stdout, -1), 12)
	})
	twin, err := s.st.h.measure(t.Context(), s.st.m12, "go", nil, "test", "-count=1", "./s/...")
	gotest.NoError(t, err)
	t.It("and its twin passes under go test", func(it *gotest.T) {
		gotest.Equal(it, 0, twin.Exit, "stderr: %s", twin.Stderr)
	})
	t.It("the shim saw the go command of the twin", func(it *gotest.T) {
		gotest.NotEmpty(it, twin.Go)
		gotest.Equal(it, []string{"test", "-count=1", "./s/..."}, twin.Go[0])
	})
	t.It("an edit changes the file", func(it *gotest.T) {
		file := filepath.Join(s.st.m12, "g", "p07", "pkg.go")
		before, err := os.ReadFile(file)
		gotest.NoError(it, err)
		gotest.NoError(it, edit(file))
		after, err := os.ReadFile(file)
		gotest.NoError(it, err)
		gotest.NotEqual(it, string(before), string(after))
	})
}

