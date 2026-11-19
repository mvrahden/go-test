package cli_test

import (
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIPhasesTestSuite: a --debug run ends by naming where its time went.
type CLIPhasesTestSuite struct{}

func (s *CLIPhasesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

var phasesLine = regexp.MustCompile(`^DEBUG: phases: discover=(\S+) generate=(\S+) compile=(\S+) run=(\S+) report=(\S+) total=(\S+)$`)

func (s *CLIPhasesTestSuite) TestWhereARunsTimeWent(t *gotest.T) {
	// The --debug run keeps its work dir; it lies in a temp dir of its own.
	debug, err := testkit.RunCLIWithEnv(t.Context(), ".", tempEnv(t.TempDir()), "--debug", "../testdata/passing/")
	gotest.NoError(t, err)
	plain, err := testkit.RunCLI(t.Context(), ".", "../testdata/passing/")
	gotest.NoError(t, err)
	lines := strings.Split(strings.TrimRight(debug.Stderr, "\n"), "\n")
	m := phasesLine.FindStringSubmatch(lines[len(lines)-1])
	t.When("a passing package is run with --debug", func(w *gotest.T) {
		w.It("the last line of stderr names the six spans as durations", func(it *gotest.T) {
			gotest.Equal(it, 0, debug.Exit, "stderr: %s", debug.Stderr)
			gotest.Len(it, m, 7, "stderr: %s", debug.Stderr)
			for _, s := range m[1:] {
				_, err := time.ParseDuration(s)
				gotest.NoError(it, err)
			}
		})
		w.It("compile is no longer than run, and the others add up to no more than the total", func(it *gotest.T) {
			gotest.Len(it, m, 7, "stderr: %s", debug.Stderr)
			d := func(i int) time.Duration { v, _ := time.ParseDuration(m[i]); return v }
			// The slack is each field's rounding to the millisecond.
			gotest.LessOrEqual(it, d(3), d(4)+time.Millisecond)
			gotest.LessOrEqual(it, d(1)+d(2)+d(4)+d(5), d(6)+4*time.Millisecond)
		})
	})
	t.When("the package is run without --debug", func(w *gotest.T) {
		w.It("stderr names no phases", func(it *gotest.T) {
			gotest.Equal(it, 0, plain.Exit, "stderr: %s", plain.Stderr)
			gotest.NotContains(it, plain.Stderr, "DEBUG: phases:")
		})
	})
}
