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

// AuditGitBaseTestSuite: --base records a commit of the checkout itself,
// without touching the checkout.
type AuditGitBaseTestSuite struct {
	dir      string
	checkout string // the base committed, the head in its worktree
	state    string // the checkout's git state before the audit
	viaGit   testkit.Run
	viaFiles testkit.Run
}

func (s *AuditGitBaseTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *AuditGitBaseTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-git-")
	gotest.NoError(t, err)
	s.dir = dir
	s.checkout = stage(t, dir, "base")
	git(t, s.checkout, "init", "-q")
	git(t, s.checkout, "add", "-A")
	git(t, s.checkout, "-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "base")
	// The head replaces the base in the worktree, uncommitted.
	entries, err := os.ReadDir(s.checkout)
	gotest.NoError(t, err)
	for _, e := range entries {
		if e.Name() != ".git" {
			gotest.NoError(t, os.RemoveAll(filepath.Join(s.checkout, e.Name())))
		}
	}
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "audit", "head"), s.checkout))
	s.state = gitState(t, s.checkout)

	s.viaGit, err = testkit.RunCLIWithEnv(t.Context(), s.checkout, outside(dir), "audit", "--base=HEAD", "--format=json")
	gotest.NoError(t, err)
	baseFile, headFile := filepath.Join(dir, "base.json"), filepath.Join(dir, "head.json")
	gotest.Equal(t, 0, record(t, stage(t, filepath.Join(dir, "plain"), "base"), baseFile).Exit)
	gotest.Equal(t, 0, record(t, stage(t, filepath.Join(dir, "plain"), "head"), headFile).Exit)
	s.viaFiles, err = testkit.RunCLI(t.Context(), dir, "audit", "--base-record="+baseFile, "--head-record="+headFile, "--format=json")
	gotest.NoError(t, err)
}

func (s *AuditGitBaseTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// git runs git in dir.
func git(t *gotest.T, dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), outside(filepath.Dir(dir))...)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// outside keeps git from finding a checkout above dir.
func outside(dir string) []string {
	return []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir)}
}

// gitState is what git says of the checkout: its status, index, worktrees,
// stash and HEAD.
func gitState(t *gotest.T, dir string) string {
	return strings.Join([]string{
		git(t, dir, "status", "--porcelain=v1", "--untracked-files=all"),
		git(t, dir, "ls-files", "--stage"),
		git(t, dir, "worktree", "list", "--porcelain"),
		git(t, dir, "stash", "list"),
		git(t, dir, "rev-parse", "HEAD"),
	}, "\n--\n")
}

func (s *AuditGitBaseTestSuite) TestABaseFromGit(t *gotest.T) {
	t.When("audit --base names a commit of the checkout", func(w *gotest.T) {
		w.It("compares that commit's tree with the working tree's, as the records of both would", func(it *gotest.T) {
			gotest.Equal(it, 0, s.viaGit.Exit, "stderr: %s", s.viaGit.Stderr)
			gotest.Equal(it, 0, s.viaFiles.Exit, "stderr: %s", s.viaFiles.Stderr)
			gotest.NotContains(it, s.viaGit.Stderr, "note: --base")
			lists := func(stdout string) string {
				var doc struct {
					Findings  json.RawMessage `json:"findings"`
					Behaviors json.RawMessage `json:"behaviors"`
				}
				gotest.NoError(it, json.Unmarshal([]byte(stdout), &doc))
				return string(doc.Findings) + "\n" + string(doc.Behaviors)
			}
			gotest.Equal(it, lists(s.viaFiles.Stdout), lists(s.viaGit.Stdout))
			doc := readDoc(it, s.viaGit.Stdout)
			gotest.Len(it, doc.Findings, 16)
			gotest.Equal(it, "HEAD", doc.Base.Ref)
			gotest.Equal(it, strings.TrimSpace(git(it, s.checkout, "rev-parse", "--short", "HEAD")), doc.Base.Commit)
		})
		w.It("leaves the checkout's status, index and worktree list as they were", func(it *gotest.T) {
			gotest.Equal(it, s.state, gitState(it, s.checkout))
			gotest.NotEmpty(it, git(it, s.checkout, "status", "--porcelain"), "the head is uncommitted")
		})
	})
	t.When("audit --base names no commit of the checkout", func(w *gotest.T) {
		w.It("exits 2 naming --base-record and --head-record", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), s.checkout, outside(s.dir), "audit", "--base=no-such-ref")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit)
			gotest.Empty(it, run.Stdout)
			gotest.Equal(it, "FAIL: --base \"no-such-ref\" names no commit of this checkout; record the base yourself and give --base-record and --head-record\n", run.Stderr)
		})
	})
}
