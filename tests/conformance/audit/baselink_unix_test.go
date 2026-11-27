//go:build unix

package audit_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditBaseLinkTestSuite: a workspace link the base's commit holds is not
// written through when the base is exported.
type AuditBaseLinkTestSuite struct {
	dir, cache, planted string
	run                 testkit.Run
}

func (s *AuditBaseLinkTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditBaseLinkTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-baselink-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = filepath.Join(dir, "cache")
	s.planted = filepath.Join(dir, "victim", "planted.conf")
	// Without the directory the write could not happen at all.
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "victim"), 0o750))
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	co := filepath.Join(dir, "checkout")
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "audit", "gates", "base"), co))
	work := filepath.Join(co, "go.work")
	data, err := os.ReadFile(work)
	gotest.NoError(t, err)
	gotest.NoError(t, os.Remove(work))
	// From <cache>/audit/<key>/tree/go.work it names dir/victim/planted.conf.
	gotest.NoError(t, os.Symlink(filepath.Join("..", "..", "..", "..", "victim", "planted.conf"), work))
	git(t, co, "init", "-q")
	git(t, co, "add", "-A")
	git(t, co, "-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "base")
	gotest.NoError(t, os.Remove(work))
	gotest.NoError(t, os.WriteFile(work, data, 0o600))
	s.run = auditIn(t, co, s.cache, "--base=HEAD", "--format=json")
}

func (s *AuditBaseLinkTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditBaseLinkTestSuite) TestALinkInTheBase(t *gotest.T) {
	t.When("audit --base names a commit whose go.work is a symbolic link to a file that does not exist, on Unix", func(t *gotest.T) {
		t.It("writes no file where the link points and exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, s.run.Exit, "stderr: %s", s.run.Stderr)
			_, err := os.Lstat(s.planted)
			gotest.ErrorIs(it, err, fs.ErrNotExist)
			matches, err := filepath.Glob(filepath.Join(s.cache, "audit", "*", "tree", "go.work"))
			gotest.NoError(it, err)
			gotest.Len(it, matches, 1)
			info, err := os.Lstat(matches[0])
			gotest.NoError(it, err)
			gotest.Equal(it, os.ModeSymlink, info.Mode()&os.ModeSymlink)
		})
	})
}
