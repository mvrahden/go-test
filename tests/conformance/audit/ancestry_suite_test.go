package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditBaseAncestryTestSuite: a base that is no ancestor of HEAD is said,
// where git can know it.
type AuditBaseAncestryTestSuite struct {
	dir, cache, side, shallowDir string
	diverged, shallow            testkit.Run
}

func (s *AuditBaseAncestryTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditBaseAncestryTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-ancestry-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = filepath.Join(dir, "cache")
	co := gatesCheckout(t, dir, "diverged")
	commit := []string{"-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "--allow-empty", "-m"}
	git(t, co, "branch", "side")
	git(t, co, append(commit, "head")...)
	git(t, co, "checkout", "-q", "side")
	git(t, co, append(commit, "side")...)
	git(t, co, "checkout", "-q", "-")
	s.side = strings.TrimSpace(git(t, co, "rev-parse", "--short", "side"))
	s.diverged = auditIn(t, co, s.cache, "--base=side", "--format=json")
	s.shallowDir = filepath.Join(dir, "shallow")
	git(t, dir, "clone", "-q", "--depth=1", "--no-local", co, s.shallowDir)
	git(t, s.shallowDir, "fetch", "-q", "--depth=1", "origin", "side:side")
	s.shallow = auditIn(t, s.shallowDir, s.cache, "--base=side", "--format=json")
}

func (s *AuditBaseAncestryTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditBaseAncestryTestSuite) TestABaseBesideHead(t *gotest.T) {
	t.When("audit --base names a commit that is no ancestor of HEAD", func(t *gotest.T) {
		t.It("prints one note on stderr naming the ref, its commit and the merge base, and compares the commit the ref names", func(it *gotest.T) {
			gotest.Equal(it, 0, s.diverged.Exit, "stderr: %s", s.diverged.Stderr)
			// Matched by content: a slow machine may add a notice of its own.
			gotest.Contains(it, s.diverged.Stderr, "note: --base side ("+s.side+") is not an ancestor of HEAD: what it holds and HEAD lacks reads as removed; for the merge base give --base=$(git merge-base side HEAD)\n")
			gotest.Equal(it, 1, strings.Count(s.diverged.Stderr, "note: --base "))
			doc := readRunDoc(it, s.diverged.Stdout)
			gotest.Equal(it, s.side, doc.Base.Commit)
		})
	})
	t.When("audit --base names a commit a shallow checkout holds no path to", func(t *gotest.T) {
		t.It("prints no note on stderr", func(it *gotest.T) {
			gotest.Equal(it, 0, s.shallow.Exit, "stderr: %s", s.shallow.Stderr)
			gotest.NotContains(it, s.shallow.Stderr, "note: --base")
			gotest.Equal(it, "true", strings.TrimSpace(git(it, s.shallowDir, "rev-parse", "--is-shallow-repository")))
		})
	})
}
