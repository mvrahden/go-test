package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditRunTestSuite: in a checkout whose head is uncommitted, audit runs
// what changed itself, reads the changed lines from git, and keeps the base
// it exports in gotest's cache.
type AuditRunTestSuite struct {
	dir, checkout, cache string
	ran, lines, clean    testkit.Run
	newDir               testkit.Run // a package directory the base lacks
	// tree is a file of the export, as each of the two audits left it.
	afterRun, afterLines os.FileInfo
	exports              []string // the cache's audit directory after both
	gone                 bool     // the export is gone after clean --cache
}

func (s *AuditRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditRunTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-run-")
	gotest.NoError(t, err)
	s.dir = dir
	s.checkout = stage(t, dir, "base")
	git(t, s.checkout, "init", "-q")
	git(t, s.checkout, "add", "-A")
	git(t, s.checkout, "-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "base")
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

	s.cache = filepath.Join(dir, "cache")
	s.ran = s.audit(t, "--base=HEAD", "--run", "--format=json")
	s.afterRun = s.exported(t)
	cover, err := filepath.Abs(filepath.Join("..", "testdata", "audit", "proof", "ledger.cover"))
	gotest.NoError(t, err)
	s.lines = s.audit(t, "--base=HEAD", "--coverage="+cover, "--format=json")
	s.afterLines = s.exported(t)
	audits, err := os.ReadDir(filepath.Join(s.cache, "audit"))
	gotest.NoError(t, err)
	for _, e := range audits {
		s.exports = append(s.exports, e.Name())
	}
	s.newDir = s.audit(t, "--base=HEAD", "--format=json", "./shop/", "./shop/ledger/")
	s.clean = s.cli(t, "clean", "--cache")
	_, err = os.Stat(filepath.Join(s.cache, "audit", strings.TrimSuffix(s.exports[0], ".lock")))
	s.gone = os.IsNotExist(err)
}

func (s *AuditRunTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditRunTestSuite) audit(t *gotest.T, args ...string) testkit.Run {
	return s.cli(t, append([]string{"audit"}, args...)...)
}

// cli runs the CLI in the checkout with the suite's own gotest cache.
func (s *AuditRunTestSuite) cli(t *gotest.T, args ...string) testkit.Run {
	run, err := testkit.RunCLIWithEnv(t.Context(), s.checkout, append(outside(s.dir), "GOTEST_CACHE_DIR="+s.cache), args...)
	gotest.NoError(t, err)
	return run
}

// exported is a file of the one export the cache holds.
func (s *AuditRunTestSuite) exported(t *gotest.T) os.FileInfo {
	matches, err := filepath.Glob(filepath.Join(s.cache, "audit", "*", "tree", "shop", "shop.go"))
	gotest.NoError(t, err)
	gotest.Len(t, matches, 1)
	info, err := os.Stat(matches[0])
	gotest.NoError(t, err)
	return info
}

func (s *AuditRunTestSuite) TestTheAuditsOwnRun(t *gotest.T) {
	t.When("audit --run compares the working tree with a base", func(w *gotest.T) {
		w.It("runs the packages of the changed behaviors and states their verdicts as current evidence of that run", func(it *gotest.T) {
			doc := readProof(it, s.ran)
			gotest.Len(it, doc.Streams, 1)
			st := doc.Streams[0]
			gotest.True(it, st.Ran)
			gotest.True(it, st.Current, st.Stale)
			gotest.NotZero(it, st.Run)
			gotest.Equal(it, strings.TrimSpace(git(it, s.checkout, "rev-parse", "--short", "HEAD")), st.Run.Commit)
			gotest.True(it, *st.Run.Dirty, "the head is uncommitted")
			// F_TestTotal narrows the run to itself.
			total := doc.evidenceOf("Cart / Total")
			gotest.Equal(it, "pass", total.Verdict)
			gotest.NotZero(it, total.Assertions)
			gotest.Equal(it, 1, *total.Assertions)
			gotest.True(it, total.Current)
			gotest.Equal(it, "not run", doc.evidenceOf("Cart / Rounds").Verdict)
			gotest.Equal(it, "skip", doc.evidenceOf("Stock").Verdict)
			gotest.Len(it, doc.Evidence, 11)
		})
		w.It("lists the changed production lines that run did not execute", func(it *gotest.T) {
			doc := readProof(it, s.ran)
			var got []unproven
			for _, u := range doc.Unproven {
				u.Ranges = nil
				got = append(got, u)
			}
			gotest.Equal(it, []unproven{
				{File: "shop/ledger/ledger.go"},
				{File: "shop/shop.go", Line: 9, Coverage: true, Changed: 7, Unexecuted: 7},
			}, got)
			shop := doc.unprovenOf("shop/shop.go")
			gotest.Len(it, shop.Ranges, 1)
			gotest.Equal(it, []int{9, 15}, []int{shop.Ranges[0].Line, shop.Ranges[0].EndLine})
		})
	})
}

func (s *AuditRunTestSuite) TestLinesFromGit(t *gotest.T) {
	t.When("audit --base with --coverage runs in a checkout with uncommitted and untracked production files", func(w *gotest.T) {
		w.It("takes the changed lines from the commit to the files on disk, every line of an untracked file included", func(it *gotest.T) {
			gotest.Contains(it, git(it, s.checkout, "status", "--porcelain"), "?? shop/ledger/")
			doc := readProof(it, s.lines)
			type file struct {
				File               string
				Changed, Unexec    int
				First, Last, Count int
			}
			var got []file
			for _, u := range doc.Unproven {
				gotest.NotEmpty(it, u.Ranges, u.File)
				got = append(got, file{u.File, u.Changed, u.Unexecuted, u.Ranges[0].Line, u.Ranges[len(u.Ranges)-1].EndLine, len(u.Ranges)})
			}
			gotest.Equal(it, []file{
				{"shop/ledger/ledger.go", 3, 3, 5, 7, 1},
				{"shop/shop.go", 7, 3, 10, 15, 2},
			}, got)
		})
	})
}

func (s *AuditRunTestSuite) TestWhereABaseIsExported(t *gotest.T) {
	t.When("audit --base records one commit twice", func(w *gotest.T) {
		w.It("exports it once into one directory under gotest's cache, outside the checkout", func(it *gotest.T) {
			gotest.Equal(it, 0, s.lines.Exit, "stderr: %s", s.lines.Stderr)
			gotest.Len(it, s.exports, 2, "%v", s.exports)
			key := strings.TrimSuffix(s.exports[0], ".lock")
			gotest.Equal(it, []string{key, key + ".lock"}, s.exports)
			gotest.True(it, os.SameFile(s.afterRun, s.afterLines), "the second audit exported again")
			gotest.Equal(it, s.afterRun.ModTime(), s.afterLines.ModTime())
			rel, err := filepath.Rel(s.checkout, s.cache)
			gotest.NoError(it, err)
			gotest.Regexp(it, `^\.\.`, rel)
		})
		w.It("gotest clean --cache removes that directory", func(it *gotest.T) {
			gotest.Equal(it, 0, s.clean.Exit, "stderr: %s", s.clean.Stderr)
			gotest.Contains(it, s.clean.Stderr, "removed 1 audit base tree from "+filepath.Join(s.cache, "audit")+"\n")
			gotest.True(it, s.gone)
		})
	})
}

func (s *AuditRunTestSuite) TestABaseWithoutADirectory(t *gotest.T) {
	t.When("audit --base names a commit that lacks a directory one of the packages names", func(w *gotest.T) {
		w.It("records no package of that directory for the base and compares the rest, exiting 0", func(it *gotest.T) {
			gotest.Equal(it, 0, s.newDir.Exit, "stderr: %s", s.newDir.Stderr)
			var doc struct {
				Incomplete []string `json:"incomplete"`
				Findings   []any    `json:"findings"`
				Base       struct{ Patterns []string }
			}
			gotest.NoError(it, json.Unmarshal([]byte(s.newDir.Stdout), &doc), s.newDir.Stdout)
			gotest.Empty(it, doc.Incomplete)
			gotest.Len(it, doc.Findings, 16)
			gotest.Equal(it, []string{"./shop/", "./shop/ledger/"}, doc.Base.Patterns)
		})
	})
}
