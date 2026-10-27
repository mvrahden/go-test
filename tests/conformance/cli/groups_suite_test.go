package cli_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIGroupsTestSuite: --group and --skip-group narrow a run to the
// ExclusiveGroups the code declares.
type CLIGroupsTestSuite struct{}

func (s *CLIGroupsTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Each run is about 3s plus its build, twice that cold under -race.
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *CLIGroupsTestSuite) TestSelection(t *gotest.T) {
	only, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/groupfixtures/", "--group=e2e")
	gotest.NoError(t, err)
	others, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/groupfixtures/", "--skip-group=e2e")
	gotest.NoError(t, err)
	t.When("--group=e2e is given", func(w *gotest.T) {
		w.It("runs only the e2e group's suites and starts only their fixtures", func(it *gotest.T) {
			ranOnly(it, &only, []string{"stack.BeforeAll", "e2e stack ran", "e2e slow ends"}, []string{"default begins", "load.BeforeAll", "load ran"})
		})
	})
	t.When("--skip-group=e2e is given", func(w *gotest.T) {
		w.It("runs every suite outside the e2e group and starts none of its fixtures", func(it *gotest.T) {
			ranOnly(it, &others, []string{"default ends", "load.BeforeAll", "load ran"}, []string{"stack.BeforeAll", "e2e stack ran", "e2e slow begins"})
		})
	})
}

// ranOnly checks that run passed, stamping every marker of ran and none of notRan.
func ranOnly(t *gotest.T, run *testkit.Run, ran, notRan []string) {
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	gotest.Subset(t, run.Markers, ran)
	for _, m := range notRan {
		gotest.NotContains(t, run.Markers, m)
	}
}

// testBinaries lists the test binaries under dir.
func testBinaries(t *gotest.T, dir string) []string {
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && regexp.MustCompile(`\.test(\.exe)?$`).MatchString(d.Name()) {
			found = append(found, path)
		}
		return nil
	})
	gotest.NoError(t, err)
	return found
}

func (s *CLIGroupsTestSuite) TestAnUnknownGroup(t *gotest.T) {
	t.When("--group or --skip-group names a group no suite declares", func(w *gotest.T) {
		w.It("exits 2 before any build, naming the known groups", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct{ Desc string }{{"--group=nope"}, {"--skip-group=nope"}}) {
				run, err := testkit.RunCLI(sub.Context(), ".", "--debug", "--no-cache", row.Desc, "../testdata/groupfixtures/")
				gotest.NoError(sub, err)
				workDir := regexp.MustCompile(`DEBUG: work dir: (\S+)`).FindStringSubmatch(run.Stderr)
				gotest.Len(sub, workDir, 2, "stderr: %s", run.Stderr)
				binaries := testBinaries(sub, workDir[1])
				gotest.NoError(sub, os.RemoveAll(workDir[1]))
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, `no suite declares the group "nope"; known groups: e2e, load`)
				gotest.Empty(sub, binaries)
			}
		})
	})
}

func (s *CLIGroupsTestSuite) TestGroupsWhereNothingRuns(t *gotest.T) {
	t.When("--group or --skip-group is given to a command that runs no suite", func(w *gotest.T) {
		w.It("exits 2, saying the command runs nothing", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc, flag string
				args       []string
			}{
				{Desc: "summary --input", flag: "--input", args: []string{"summary", "--input=events.json", "--group=e2e"}},
				{Desc: "spec --input", flag: "--input", args: []string{"spec", "--input=events.json", "--skip-group=e2e"}},
				{Desc: "spec --static", flag: "--static", args: []string{"spec", "--static", "--group=e2e", "../testdata/groupfixtures/"}},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, "--group and --skip-group select the suites a run runs, and "+row.flag+" runs nothing")
			}
		})
	})
}
