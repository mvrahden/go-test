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

// AuditCurrencyTestSuite: a stream that stamped its verdicts with input
// hashes is judged by them, not by the tree's commit.
type AuditCurrencyTestSuite struct {
	dir, head string
	doc       currencyDoc
}

func (s *AuditCurrencyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: time.Minute} // a staged module run once and audited
}

// currencyDoc is the part of the audit document the suite reads.
type currencyDoc struct {
	Streams []struct {
		Side    string `json:"side"`
		Current bool   `json:"current"`
		Stale   string `json:"stale"`
	} `json:"streams"`
	Evidence []struct {
		Package string `json:"package"`
		Current bool   `json:"current"`
		Stale   string `json:"stale"`
		Stream  *int   `json:"stream"`
		Others  []struct {
			Stream int `json:"stream"`
		} `json:"others"`
	} `json:"evidence"`
}

// BeforeAll runs the head's three packages once, saves the stream whole and
// with fresh/c's events alone, edits fresh/b and adds an untracked file, and
// audits the head given the whole stream, then the fresh/c one, and the
// whole stream again as the base's run.
func (s *AuditCurrencyTestSuite) BeforeAll(t *gotest.T) {
	parent, err := os.MkdirTemp("", "gotest-audit-currency-")
	gotest.NoError(t, err)
	s.dir = parent
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	base, head := filepath.Join(parent, "base"), filepath.Join(parent, "head")
	s.head = head
	for _, dir := range []string{base, head} {
		gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "fresh"), dir))
	}
	// The base declares no suite, so each of the head's is a change.
	tests, err := filepath.Glob(filepath.Join(base, "*", "*_suite_test.go"))
	gotest.NoError(t, err)
	for _, f := range tests {
		gotest.NoError(t, os.Remove(f))
	}
	baseRecord := filepath.Join(parent, "base.json")
	gotest.Equal(t, 0, record(t, base, baseRecord).Exit)
	git(t, head, "-c", "init.defaultBranch=trunk", "init", "-q")
	git(t, head, "add", "-A")
	git(t, head, "-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "head")
	env := append(outside(parent), "GOTEST_CACHE_DIR="+filepath.Join(parent, "cache"))
	stream, err := testkit.RunCLIWithEnv(t.Context(), head, env, "-json", "./a/", "./b/", "./c/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, stream.Exit, "stderr: %s", stream.Stderr)
	whole, onlyC := filepath.Join(parent, "events.json"), filepath.Join(parent, "c.json")
	gotest.NoError(t, os.WriteFile(whole, []byte(stream.Stdout), 0o600))
	var kept []string
	for _, line := range strings.SplitAfter(stream.Stdout, "\n") {
		if !strings.Contains(line, `"Package":"fresh/a"`) && !strings.Contains(line, `"Package":"fresh/b"`) {
			kept = append(kept, line)
		}
	}
	gotest.NoError(t, os.WriteFile(onlyC, []byte(strings.Join(kept, "")), 0o600))
	b := filepath.Join(head, "b", "b.go")
	src, err := os.ReadFile(b)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(b, append(src, "\n// edited\n"...), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(head, "notes", "new.txt"), []byte("untracked\n"), 0o600))
	run, err := testkit.RunCLIWithEnv(t.Context(), head, env, "audit", "--base-record="+baseRecord,
		"--evidence="+whole, "--evidence="+onlyC, "--base-evidence="+whole, "--format=json", "./a/", "./b/", "./c/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &s.doc), "document: %s", run.Stdout)
	gotest.Len(t, s.doc.Streams, 3, "document: %s", run.Stdout)
}

func (s *AuditCurrencyTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// evidenceOf maps each package to its evidence row's currency.
func (s *AuditCurrencyTestSuite) evidenceOf() (current map[string]bool, stale map[string]string) {
	current, stale = map[string]bool{}, map[string]string{}
	for _, e := range s.doc.Evidence {
		current[e.Package] = e.Current
		stale[e.Package] = e.Stale
	}
	return current, stale
}

func (s *AuditCurrencyTestSuite) TestAStreamWithInputHashes(t *gotest.T) {
	current, stale := s.evidenceOf()
	t.When("audit is given a stream with input hashes after an uncommitted change to one of its packages and to a file no package reads", func(w *gotest.T) {
		w.It("marks the evidence of the unchanged package current, though the tree has uncommitted changes", func(it *gotest.T) {
			gotest.Contains(it, git(it, s.head, "status", "--porcelain"), "notes/")
			gotest.True(it, current["fresh/c"], "evidence: %+v", s.doc.Evidence)
			gotest.Empty(it, stale["fresh/c"])
		})
		w.It("marks the evidence of the changed package stale, naming its inputs changed", func(it *gotest.T) {
			gotest.False(it, current["fresh/b"])
			gotest.Equal(it, "the inputs of fresh/b changed since the stream ran", stale["fresh/b"])
			gotest.Equal(it, "the inputs of fresh/a changed since the stream ran", stale["fresh/a"])
		})
	})
}

func (s *AuditCurrencyTestSuite) TestAStreamsOwnCurrency(t *gotest.T) {
	t.When("audit is given streams with input hashes, one also as the base's run, after an uncommitted change to a package one of them ran", func(w *gotest.T) {
		w.It("marks the stream that ran the changed package stale, naming each package whose inputs changed", func(it *gotest.T) {
			st := s.doc.Streams[0]
			gotest.Equal(it, "head", st.Side)
			gotest.False(it, st.Current)
			gotest.Equal(it, "the inputs changed since the stream ran: fresh/a, fresh/b", st.Stale)
		})
		w.It("marks the stream of one unchanged package current, though the tree has uncommitted changes", func(it *gotest.T) {
			st := s.doc.Streams[1]
			gotest.Equal(it, "head", st.Side)
			gotest.True(it, st.Current, st.Stale)
		})
		w.It("judges the base's stream by commit", func(it *gotest.T) {
			st := s.doc.Streams[2]
			gotest.Equal(it, "base", st.Side)
			gotest.False(it, st.Current)
			gotest.Equal(it, "the base names no commit", st.Stale)
		})
	})
}

func (s *AuditCurrencyTestSuite) TestTwoCurrentStreams(t *gotest.T) {
	t.When("two current streams judge one behavior", func(w *gotest.T) {
		w.It("states the verdict of the first given and names the other's among the others", func(it *gotest.T) {
			var rows int
			for _, e := range s.doc.Evidence {
				if e.Package != "fresh/c" {
					continue
				}
				rows++
				gotest.True(it, e.Current)
				gotest.NotZero(it, e.Stream)
				gotest.Equal(it, 0, *e.Stream)
				gotest.Len(it, e.Others, 1)
				gotest.Equal(it, 1, e.Others[0].Stream)
			}
			gotest.NotZero(it, rows, "evidence: %+v", s.doc.Evidence)
		})
	})
}
