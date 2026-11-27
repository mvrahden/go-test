package audit_test

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

// AuditChildCoverageTestSuite: audit --run counts what the binaries its
// tests start execute, where they were built with -cover. The checkout's
// commit holds one package and greet without its empty name's branch; the
// binaries, plain and the tests that start them are untracked, so each of
// their lines is changed. The CLI is built with -cover, which its run must
// not count as a binary a test started.
type AuditChildCoverageTestSuite struct {
	dir, checkout, tmp, cli string
	// status is the checkout's git status before the audits.
	status string
	// both runs every test, plain only the one that starts the plain binary.
	both, plain testkit.Run
	// left is what the run left under GOTMPDIR after both.
	left []string
}

func (s *AuditChildCoverageTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditChildCoverageTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-childcov-")
	gotest.NoError(t, err)
	s.dir = dir
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.checkout = filepath.Join(dir, "childcov")
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "audit", "childcov"), s.checkout))
	git(t, s.checkout, "init", "-q")
	// The base's greet lacks the branch, so its func line is unchanged:
	// whether a block spans that line depends on the toolchain.
	greet := filepath.Join(s.checkout, "greet", "greet.go")
	head, err := os.ReadFile(greet)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(greet, []byte(greetBase), 0o600))
	git(t, s.checkout, "add", "go.mod", "go.work", "version", "greet")
	git(t, s.checkout, "-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "base")
	gotest.NoError(t, os.WriteFile(greet, head, 0o600))
	s.status = git(t, s.checkout, "status", "--porcelain", "--untracked-files=all")
	s.tmp = filepath.Join(dir, "tmp")
	gotest.NoError(t, os.Mkdir(s.tmp, 0o700))
	gotest.NoError(t, os.Mkdir(filepath.Join(dir, "clicover"), 0o700))
	s.cli = filepath.Join(dir, "gotest"+filepath.Ext(os.Args[0]))
	build := exec.CommandContext(t.Context(), "go", "build", "-cover", "-o", s.cli, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	build.Dir = root
	out, err := build.CombinedOutput()
	gotest.NoError(t, err, "%s", out)

	s.both = s.audit(t, "--format=json")
	s.left = s.leftover(t)
	s.plain = s.audit(t, "--format=json", "./plain/", "./cmd/plain/", "./e2e/plain/")
}

func (s *AuditChildCoverageTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// audit runs the suite's CLI's audit --run in the checkout, under env and
// GOTMPDIR, where the run makes its directory.
func (s *AuditChildCoverageTestSuite) audit(t *gotest.T, args ...string) testkit.Run {
	env := append(s.env(), "GOTMPDIR="+s.tmp)
	run, err := testkit.RunBinaryWithEnv(t.Context(), s.cli, s.checkout, env, append([]string{"audit", "--base=HEAD", "--run"}, args...)...)
	gotest.NoError(t, err)
	return run
}

// env is the audits' environment: alone under git, with the suite's own
// cache, and GOCOVERDIR for the CLI's own counters.
func (s *AuditChildCoverageTestSuite) env() []string {
	return append(outside(s.dir), "GOTEST_CACHE_DIR="+filepath.Join(s.dir, "cache"), "GOCOVERDIR="+filepath.Join(s.dir, "clicover"))
}

// greetBase is greet.go at the base commit: Hello without its empty name's
// branch.
const greetBase = `// Package greet is run only by the hello binary.
package greet

// Hello greets name.
func Hello(name string) string {
	return "hello, " + name
}
`

// leftover lists what GOTMPDIR holds.
func (s *AuditChildCoverageTestSuite) leftover(t *gotest.T) []string {
	entries, err := os.ReadDir(s.tmp)
	gotest.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// childDoc is the part of the audit document the suite reads.
type childDoc struct {
	Unproven   []unproven `json:"unproven"`
	Notes      []string   `json:"notes"`
	Incomplete []string   `json:"incomplete"`
}

func readChildDoc(t *gotest.T, run testkit.Run, exit int) childDoc { //nolint:gocritic // hugeParam: a run as RunCLI returns it
	gotest.Equal(t, exit, run.Exit, "stderr: %s", run.Stderr)
	var doc childDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	return doc
}

// files maps each unproven file to its coverage, changed, unexecuted
// count and first and last unexecuted line.
func (d *childDoc) files() map[string][5]int {
	out := map[string][5]int{}
	for _, u := range d.Unproven {
		row := [5]int{0, u.Changed, u.Unexecuted}
		if u.Coverage {
			row[0] = 1
		}
		if len(u.Ranges) > 0 {
			row[3], row[4] = u.Ranges[0].Line, u.Ranges[len(u.Ranges)-1].EndLine
		}
		out[u.File] = row
	}
	return out
}

const childCoverNote = "--run: no binary a test started wrote coverage data; one counts when built with go build -cover (https://github.com/mvrahden/go-test/blob/main/specs/audit/spec.md#requirement-coverage-of-the-binaries-a-test-starts)"

func (s *AuditChildCoverageTestSuite) TestABinaryBuiltWithCover(t *gotest.T) {
	t.When("audit --run runs a test that starts a binary built with go build -cover", func(w *gotest.T) {
		w.It("lists, of the changed lines that binary's packages hold, only those it did not execute", func(it *gotest.T) {
			doc := readChildDoc(it, s.both, 0)
			// hello runs greet.Hello with a name: of greet.go's changed
			// lines 6 to 8, all but the empty name's branch on 7 and 8; and
			// all of cmd/hello/main.go, which is therefore not listed.
			gotest.Equal(it, map[string][5]int{
				"greet/greet.go":    {1, 3, 2, 7, 8},
				"plain/plain.go":    {0, 0, 0, 0, 0},
				"cmd/plain/main.go": {0, 0, 0, 0, 0},
			}, doc.files())
			gotest.Contains(it, doc.Notes, "--run: the coverage counts what 1 binary the tests started executed")
			gotest.NotContains(it, doc.Notes, childCoverNote)
		})
		w.It("leaves the module as it was and no directory of the run behind", func(it *gotest.T) {
			gotest.Equal(it, s.status, git(it, s.checkout, "status", "--porcelain", "--untracked-files=all"))
			gotest.Empty(it, s.left)
		})
	})
}

func (s *AuditChildCoverageTestSuite) TestBinariesBuiltWithoutCover(t *gotest.T) {
	t.When("audit --run runs only tests that start binaries built without -cover", func(w *gotest.T) {
		w.It("lists the changed files only those binaries execute as without coverage data", func(it *gotest.T) {
			doc := readChildDoc(it, s.plain, 0)
			gotest.Equal(it, map[string][5]int{
				"plain/plain.go":    {0, 0, 0, 0, 0},
				"cmd/plain/main.go": {0, 0, 0, 0, 0},
			}, doc.files())
		})
		w.It("notes that a binary a test starts counts when built with go build -cover", func(it *gotest.T) {
			doc := readChildDoc(it, s.plain, 0)
			gotest.Contains(it, doc.Notes, childCoverNote)
		})
	})
}

func (s *AuditChildCoverageTestSuite) TestAnInterruptedAuditRun(t *gotest.T) {
	// The plain test says it is under way, then waits to be stopped.
	hold := t.TempDir()
	tmp := filepath.Join(s.dir, "tmp-interrupted")
	gotest.NoError(t, os.Mkdir(tmp, 0o700))
	cmd := exec.Command(s.cli, "audit", "--base=HEAD", "--run", "--format=json", "./plain/", "./cmd/plain/", "./e2e/plain/") //nolint:gosec // G204: binary built by this suite
	cmd.Dir = s.checkout
	cmd.Env = testkit.ChildEnv(t.TempDir(), append(s.env(), "GOTMPDIR="+tmp, "CHILDCOV_HOLD="+hold)...)
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	ready := false
	for deadline := time.Now().Add(testkit.Scaled(20 * time.Second)); !ready && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		_, err := os.Stat(filepath.Join(hold, "ready"))
		ready = err == nil
	}
	if ready {
		err = testkit.Interrupt(cmd, tree)
	} else {
		err = tree.Kill()
	}
	_ = cmd.Wait()
	tree.Release()
	gotest.True(t, ready, "the run never got under way:\n%s%s", cmd.Stdout, cmd.Stderr)
	gotest.NoError(t, err)
	run := testkit.Run{Exit: cmd.ProcessState.ExitCode(), Stdout: cmd.Stdout.(*strings.Builder).String(), Stderr: cmd.Stderr.(*strings.Builder).String()}
	entries, err := os.ReadDir(tmp)
	gotest.NoError(t, err)

	t.When("audit --run is interrupted while its run is under way", func(w *gotest.T) {
		w.It("writes the document naming the stopped run among what was not compared and exits 130", func(it *gotest.T) {
			doc := readChildDoc(it, run, 130)
			gotest.Equal(it, []string{"--run: the run was stopped before its end; what it did not reach reads as not run"}, doc.Incomplete)
		})
		w.It("leaves no directory of the run behind", func(it *gotest.T) {
			gotest.Empty(it, entries)
		})
	})
}
