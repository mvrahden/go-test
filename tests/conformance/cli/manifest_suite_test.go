package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRunManifestTestSuite: a run opens its stream saying what it was and
// closes it saying what it judged.
type CLIRunManifestTestSuite struct{}

func (s *CLIRunManifestTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

// runEvent is an event of the run's own package, as a consumer decodes it.
type runEvent struct {
	Action   string
	Package  string
	RunLevel bool
	Manifest *struct {
		Version   string
		GoVersion string
		Args      []string
		Commit    string
		Dirty     *bool
		Agent     string
	}
	Result *struct {
		ExitCode int
		Packages struct{ Passed, Failed, Skipped int }
		Tests    struct{ Passed, Failed, Skipped int }
		NotRun   *int
		Vacuous  *int
	}
}

// streamEnds are the first and the last event of a stream.
func streamEnds(t *gotest.T, stdout string) (first, last runEvent) {
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	gotest.NotEmpty(t, lines)
	gotest.NoError(t, json.Unmarshal([]byte(lines[0]), &first), "first line: %s", lines[0])
	gotest.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last), "last line: %s", lines[len(lines)-1])
	return first, last
}

// commandOutput is what name prints in dir, trimmed.
func commandOutput(t *gotest.T, dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	gotest.NoError(t, err, "%s %v", name, args)
	return strings.TrimSpace(string(out))
}

// stagedModule stages the passing and the failing package as one module in
// a fresh dir and returns the dir.
func stagedModule(t *gotest.T) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "passing"), dir, "manifestmod")
	gotest.NoError(t, err)
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "failing"), dir, "manifestmod")
	gotest.NoError(t, err)
	return dir
}

// committed makes dir a git checkout with everything in it committed.
func committed(t *gotest.T, dir string) {
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "start"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		gotest.NoError(t, err, "git %v: %s", args, out)
	}
}

// outsideCheckout keeps git from finding a checkout above dir.
func outsideCheckout(dir string) []string {
	return []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir)}
}

func (s *CLIRunManifestTestSuite) TestARunInACheckout(t *gotest.T) {
	type row struct {
		Desc  string
		dirty bool
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "a clean checkout"},
		{Desc: "a checkout with uncommitted changes", dirty: true},
	}) {
		dir := stagedModule(t)
		committed(t, dir)
		if r.dirty {
			gotest.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("draft\n"), 0o600))
		}
		head := commandOutput(t, dir, "git", "rev-parse", "HEAD")
		goVersion := commandOutput(t, dir, "go", "env", "GOVERSION")
		version, err := testkit.RunCLI(t.Context(), dir, "version")
		gotest.NoError(t, err)
		run, err := testkit.RunCLI(t.Context(), dir, "-json", "./passing/")
		gotest.NoError(t, err)
		gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
		first, _ := streamEnds(t, run.Stdout)

		t.When("a -json run is made in a checkout", func(w *gotest.T) {
			w.It("opens the stream with a RunLevel start of gotest run naming the CLI's version, the go command's version and the run's arguments", func(it *gotest.T) {
				gotest.Equal(it, "start", first.Action)
				gotest.Equal(it, "gotest run", first.Package)
				gotest.True(it, first.RunLevel)
				gotest.NotZero(it, first.Manifest)
				gotest.NotEmpty(it, first.Manifest.Version)
				gotest.Contains(it, version.Stdout, " "+first.Manifest.Version+" ")
				gotest.Equal(it, goVersion, first.Manifest.GoVersion)
				gotest.Equal(it, []string{"-json", "./passing/"}, first.Manifest.Args)
			})
			w.It("names the checkout's commit and whether it has uncommitted changes", func(it *gotest.T) {
				gotest.NotZero(it, first.Manifest)
				gotest.GreaterOrEqual(it, len(first.Manifest.Commit), 7)
				gotest.Regexp(it, "^"+first.Manifest.Commit, head)
				gotest.NotZero(it, first.Manifest.Dirty)
				gotest.Equal(it, r.dirty, *first.Manifest.Dirty)
			})
		})
	}
}

func (s *CLIRunManifestTestSuite) TestARunOutsideACheckout(t *gotest.T) {
	dir := stagedModule(t)
	run, err := testkit.RunCLIWithEnv(t.Context(), dir, outsideCheckout(dir), "-json", "./passing/")
	gotest.NoError(t, err)
	first, last := streamEnds(t, run.Stdout)

	t.When("a -json run is made outside a checkout", func(w *gotest.T) {
		w.It("names no commit and no dirty flag, and exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.NotZero(it, first.Manifest)
			gotest.Empty(it, first.Manifest.Commit)
			gotest.Zero(it, first.Manifest.Dirty)
			gotest.NotZero(it, last.Result)
			gotest.Equal(it, 0, last.Result.ExitCode)
		})
	})
}

func (s *CLIRunManifestTestSuite) TestARunUnderAnAgent(t *gotest.T) {
	agentOf := func(t *gotest.T, env ...string) string {
		run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), env, "../testdata/passing/")
		gotest.NoError(t, err)
		gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
		first, _ := streamEnds(t, run.Stdout)
		gotest.NotZero(t, first.Manifest)
		return first.Manifest.Agent
	}

	t.When("a -json run is made under an agent", func(w *gotest.T) {
		w.It("names the agent AI_AGENT names, and claude-code under CLAUDECODE=1 alone", func(it *gotest.T) {
			gotest.Equal(it, "my-agent", agentOf(it, "AI_AGENT=my-agent", "CLAUDECODE=1"))
			gotest.Equal(it, "claude-code", agentOf(it, "CLAUDECODE=1"))
		})
		w.It("names no agent without either", func(it *gotest.T) {
			gotest.Empty(it, agentOf(it))
		})
	})
}

func (s *CLIRunManifestTestSuite) TestTheEndOfARun(t *gotest.T) {
	mixed, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failing/", "../testdata/passing/")
	gotest.NoError(t, err)
	passing, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/passing/")
	gotest.NoError(t, err)

	t.When("a -json run of a failing and a passing package ends", func(w *gotest.T) {
		w.It("closes the stream with a fail of gotest run carrying exit code 1, one package passed and one failed, two tests passed and two failed", func(it *gotest.T) {
			gotest.Equal(it, 1, mixed.Exit, "stderr: %s", mixed.Stderr)
			_, last := streamEnds(it, mixed.Stdout)
			gotest.Equal(it, "fail", last.Action)
			gotest.Equal(it, "gotest run", last.Package)
			gotest.True(it, last.RunLevel)
			gotest.NotZero(it, last.Result)
			gotest.Equal(it, 1, last.Result.ExitCode)
			gotest.Equal(it, struct{ Passed, Failed, Skipped int }{1, 1, 0}, last.Result.Packages)
			gotest.Equal(it, struct{ Passed, Failed, Skipped int }{2, 2, 0}, last.Result.Tests)
		})
		w.It("a passing run closes it with a pass carrying exit code 0 and 0 units not run", func(it *gotest.T) {
			gotest.Equal(it, 0, passing.Exit, "stderr: %s", passing.Stderr)
			_, last := streamEnds(it, passing.Stdout)
			gotest.Equal(it, "pass", last.Action)
			gotest.Equal(it, "gotest run", last.Package)
			gotest.NotZero(it, last.Result)
			gotest.Equal(it, 0, last.Result.ExitCode)
			gotest.NotZero(it, last.Result.NotRun)
			gotest.Equal(it, 0, *last.Result.NotRun)
		})
	})
}

func (s *CLIRunManifestTestSuite) TestARunTheCensusDoesNotJudge(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/passing/", "-run", "TestPassingTestSuite")
	gotest.NoError(t, err)

	t.When("a -json run is narrowed with -run", func(w *gotest.T) {
		w.It("closes the stream with no count of units not run", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			_, last := streamEnds(it, run.Stdout)
			gotest.NotZero(it, last.Result)
			gotest.Zero(it, last.Result.NotRun)
		})
	})
}

// documentRun is the run a spec or summary JSON document names.
type documentRun struct {
	Run *struct {
		Version   string   `json:"version"`
		GoVersion string   `json:"goVersion"`
		Args      []string `json:"args"`
		Commit    string   `json:"commit"`
		Dirty     *bool    `json:"dirty"`
		Agent     string   `json:"agent"`
		ExitCode  *int     `json:"exitCode"`
	} `json:"run"`
}

func decodeRun(t *gotest.T, stdout string) documentRun {
	var doc documentRun
	gotest.NoError(t, json.Unmarshal([]byte(stdout), &doc), "stdout: %s", stdout)
	return doc
}

func (s *CLIRunManifestTestSuite) TestTheRunADocumentRenders(t *gotest.T) {
	dir := stagedModule(t)
	committed(t, dir)
	head := commandOutput(t, dir, "git", "rev-parse", "HEAD")
	goVersion := commandOutput(t, dir, "go", "env", "GOVERSION")
	events := filepath.Join(t.TempDir(), "events.json")
	spec, err := testkit.RunCLI(t.Context(), dir, "spec", "--format=json", "./failing/")
	gotest.NoError(t, err)
	summary, err := testkit.RunCLI(t.Context(), dir, "summary", "--format=json", "--events-file="+events, "./failing/")
	gotest.NoError(t, err)
	replay, err := testkit.RunCLI(t.Context(), dir, "summary", "--format=json", "--input="+events)
	gotest.NoError(t, err)

	t.When("spec --format=json renders a live run of a failing package", func(w *gotest.T) {
		w.It("its run names the commit, the go command's version and exit code 1, after version", func(it *gotest.T) {
			for _, live := range []testkit.Run{spec, summary} {
				gotest.Equal(it, 1, live.Exit, "stderr: %s", live.Stderr)
				gotest.Regexp(it, `^\{"version":"[^"]*","run":\{`, live.Stdout)
				doc := decodeRun(it, live.Stdout)
				gotest.NotZero(it, doc.Run)
				gotest.GreaterOrEqual(it, len(doc.Run.Commit), 7)
				gotest.Regexp(it, "^"+doc.Run.Commit, head)
				gotest.Equal(it, goVersion, doc.Run.GoVersion)
				gotest.NotZero(it, doc.Run.ExitCode)
				gotest.Equal(it, 1, *doc.Run.ExitCode)
			}
		})
		w.It("summary --format=json of the run's saved stream names the same run", func(it *gotest.T) {
			gotest.Equal(it, 1, replay.Exit, "stderr: %s", replay.Stderr)
			live, saved := decodeRun(it, summary.Stdout), decodeRun(it, replay.Stdout)
			gotest.NotZero(it, saved.Run)
			gotest.Equal(it, live.Run, saved.Run)
			gotest.Equal(it, []string{"summary", "--format=json", "--events-file=" + events, "./failing/"}, saved.Run.Args)
		})
	})
}

// goTestStream is a stream as go test -json writes it.
const goTestStream = `{"Action":"start","Package":"example.com/plain"}
{"Action":"run","Package":"example.com/plain","Test":"TestPlain"}
{"Action":"pass","Package":"example.com/plain","Test":"TestPlain","Elapsed":0}
{"Action":"pass","Package":"example.com/plain","Elapsed":0.01}
`

func (s *CLIRunManifestTestSuite) TestADocumentOfGoTestsOwnStream(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "plain.json")
	gotest.NoError(t, os.WriteFile(path, []byte(goTestStream), 0o600))
	run, err := testkit.RunCLI(t.Context(), ".", "spec", "--format=json", "--input="+path)
	gotest.NoError(t, err)

	t.When("spec --format=json renders a stream go test wrote", func(w *gotest.T) {
		w.It("holds no run", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^\{"version":"[^"]*","packages":`, run.Stdout)
			gotest.Zero(it, decodeRun(it, run.Stdout).Run)
		})
	})
}
