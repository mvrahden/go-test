package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIOutputFileTestSuite: a report --output writes holds no color codes,
// even when stdout is a terminal. It runs the CLI with stdout on the null
// device, a character device on Linux and macOS.
type CLIOutputFileTestSuite struct {
	cli string
}

func (s *CLIOutputFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *CLIOutputFileTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *CLIOutputFileTestSuite) AfterAll(t *gotest.T) {}

func (s *CLIOutputFileTestSuite) TestAReportWrittenToAFile(t *gotest.T) {
	t.When("spec or summary writes its report to a file --output names while stdout is a character device", func(w *gotest.T) {
		w.It("the file holds no color codes", func(it *gotest.T) {
			for row, sub := range gotest.Each(it, []struct{ Desc string }{{Desc: "spec"}, {Desc: "summary"}}) {
				out := filepath.Join(row.TempDir(), "report.txt")
				cmd := exec.CommandContext(row.Context(), s.cli, sub.Desc, "--input=../testdata/specstreams/failed.jsonl", "--output="+out)
				cmd.Env = append(testkit.ChildEnv(row.TempDir()), "NO_COLOR=")
				null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				gotest.NoError(row, err)
				cmd.Stdout = null
				runErr := cmd.Run()
				gotest.NoError(row, null.Close())
				exit := gotest.ErrorAs[*exec.ExitError](row, runErr)
				gotest.Equal(row, 1, exit.ExitCode(), "a failing stream")
				file, err := os.ReadFile(out)
				gotest.NoError(row, err)
				gotest.NotEmpty(row, file)
				gotest.NotContains(row, string(file), "\x1b[")
			}
		})
	})
}
