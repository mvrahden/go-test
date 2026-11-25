package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditCurrencyTestSuite: a stream that stamped its verdicts with input
// hashes is judged by them, not by the tree's commit.
type AuditCurrencyTestSuite struct{}

func (s *AuditCurrencyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute} // a staged module run once and audited
}

// currencyDoc is the part of the audit document the suite reads.
type currencyDoc struct {
	Evidence []struct {
		Package string `json:"package"`
		Current bool   `json:"current"`
		Stale   string `json:"stale"`
	} `json:"evidence"`
}

func (s *AuditCurrencyTestSuite) TestAStreamWithInputHashes(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	parent := t.TempDir()
	base, head := filepath.Join(parent, "base"), filepath.Join(parent, "head")
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
	env := append(outside(parent), "GOTEST_CACHE_DIR="+t.TempDir())
	stream, err := testkit.RunCLIWithEnv(t.Context(), head, env, "-json", "./a/", "./b/", "./c/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, stream.Exit, "stderr: %s", stream.Stderr)
	file := filepath.Join(parent, "events.json")
	gotest.NoError(t, os.WriteFile(file, []byte(stream.Stdout), 0o600))
	b := filepath.Join(head, "b", "b.go")
	src, err := os.ReadFile(b)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(b, append(src, "\n// edited\n"...), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(head, "notes", "new.txt"), []byte("untracked\n"), 0o600))
	run, err := testkit.RunCLIWithEnv(t.Context(), head, env, "audit", "--base-record="+baseRecord, "--evidence="+file, "--format=json", "./a/", "./b/", "./c/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	var doc currencyDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	current, stale := map[string]bool{}, map[string]string{}
	for _, e := range doc.Evidence {
		current[e.Package] = e.Current
		stale[e.Package] = e.Stale
	}

	t.When("audit is given a stream with input hashes after an uncommitted change to one of its packages and to a file no package reads", func(w *gotest.T) {
		w.It("marks the evidence of the unchanged package current, though the tree has uncommitted changes", func(it *gotest.T) {
			gotest.Contains(it, git(it, head, "status", "--porcelain"), "notes/")
			gotest.True(it, current["fresh/c"], "evidence: %+v", doc.Evidence)
			gotest.Empty(it, stale["fresh/c"])
		})
		w.It("marks the evidence of the changed package stale, naming its inputs changed", func(it *gotest.T) {
			gotest.False(it, current["fresh/b"])
			gotest.Equal(it, "the inputs of fresh/b changed since the stream ran", stale["fresh/b"])
			gotest.Equal(it, "the inputs of fresh/a changed since the stream ran", stale["fresh/a"])
		})
	})
}
