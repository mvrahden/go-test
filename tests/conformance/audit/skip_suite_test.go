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

// AuditSkipTestSuite: a call that skips the test, added to a body, weakens
// the estate.
type AuditSkipTestSuite struct {
	dir, cache string
	// static compares the head; staticAll with --fail-on=all; oldBase
	// against a base record without skip counts.
	static, staticAll, oldBase testkit.Run
	// ran runs the head's tests.
	ran testkit.Run
}

func (s *AuditSkipTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditSkipTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-skip-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = filepath.Join(dir, "cache")
	co := gatesCheckout(t, dir, "skip", "skip")
	s.static = auditIn(t, co, s.cache, "--base=HEAD", "--format=json")
	s.staticAll = auditIn(t, co, s.cache, "--base=HEAD", "--fail-on=all")
	s.ran = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--format=json")

	plain := gatesCheckout(t, dir, "plain")
	baseFile := filepath.Join(dir, "base.json")
	rec, err := testkit.RunCLI(t.Context(), plain, "audit", "--record", "--output="+baseFile)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, rec.Exit, "stderr: %s", rec.Stderr)
	data, err := os.ReadFile(baseFile)
	gotest.NoError(t, err)
	var doc map[string]any
	gotest.NoError(t, json.Unmarshal(data, &doc))
	notes, _ := doc["notes"].([]any)
	kept, dropped := []any{}, 0
	for _, n := range notes {
		if text, _ := n.(string); strings.HasPrefix(text, "skips counts") {
			dropped++
			continue
		}
		kept = append(kept, n)
	}
	gotest.Equal(t, 1, dropped)
	doc["notes"] = kept
	data, err = json.Marshal(doc)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(baseFile, data, 0o600))
	s.oldBase = auditIn(t, co, s.cache, "--base-record="+baseFile, "--fail-on=all", "--format=json")
}

func (s *AuditSkipTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditSkipTestSuite) TestASkipTheHeadAdds(t *gotest.T) {
	t.When("the head adds calls that skip the test to two bodies and adds a test whose body writes one", func(t *gotest.T) {
		t.It("lists each as a finding of class skip naming the calls its body wrote before and writes now", func(it *gotest.T) {
			doc := readRunDoc(it, s.static.Stdout)
			gotest.Equal(it, []finding{
				{"skip", "Calc / Sub", "no skip call", "1 skip call"},
				{"skip", "Calc / SubLater", "", "1 skip call"},
				{"skip", "Calc / SubNever", "no skip call", "2 skip calls"},
			}, doc.findings(true))
		})
		t.It("--fail-on=all exits 1 and the closing line counts the three skip findings", func(it *gotest.T) {
			gotest.Equal(it, 1, s.staticAll.Exit, "stderr: %s", s.staticAll.Stderr)
			gotest.Contains(it, s.staticAll.Stdout, "gotest audit: FAIL exit 1 | weakenings: 3 (skip 3) | behaviors: 1 added, 0 removed, 0 reworded, 0 moved, 3 changed | fail-on: removed,excluded,skip,focus,guard,budget,suppression,snapshot,assertions,asserts-by\n")
			gotest.Contains(it, s.staticAll.Stdout, "\n  skip         ./calc Calc / Sub: no skip call → 1 skip call (calc/calc_suite_test.go:7)\n")
		})
	})
	t.When("the head changes a body that wrote a skip call and still writes one", func(t *gotest.T) {
		t.It("lists no skip finding for it", func(it *gotest.T) {
			doc := readRunDoc(it, s.static.Stdout)
			var changed, skipped []string
			for _, f := range doc.Findings {
				switch f.Class {
				case "changed":
					changed = append(changed, f.Subject)
				case "skip":
					skipped = append(skipped, f.Subject)
				}
			}
			gotest.Contains(it, changed, "Calc / SubOnce")
			gotest.NotContains(it, skipped, "Calc / SubOnce")
		})
	})
	t.When("the base record was written without skip counts", func(t *gotest.T) {
		t.It("lists no skip finding and notes that a skip the head adds is not listed", func(it *gotest.T) {
			gotest.Equal(it, 0, s.oldBase.Exit, "stderr: %s", s.oldBase.Stderr)
			doc := readRunDoc(it, s.oldBase.Stdout)
			gotest.Empty(it, doc.findings(true))
			gotest.Equal(it, []string{"a record does not count skip calls, so a skip the head adds is not listed; record both sides with this gotest"}, doc.Notes)
		})
	})
}

func (s *AuditSkipTestSuite) TestATestTheHeadMakesSkip(t *gotest.T) {
	t.When("audit --run runs tests the head added skip calls to", func(t *gotest.T) {
		t.It("reads each as skip and counts it as not run, beside its skip finding", func(it *gotest.T) {
			gotest.Equal(it, 0, s.ran.Exit, "stderr: %s", s.ran.Stderr)
			doc := readRunDoc(it, s.ran.Stdout)
			rows := map[string][2]any{}
			for _, e := range doc.Evidence {
				rows[strings.Join(e.Path, " / ")] = [2]any{e.Verdict, e.NeedsDecision}
			}
			for _, path := range []string{"Calc / Sub", "Calc / SubLater", "Calc / SubNever"} {
				gotest.Equal(it, [2]any{"skip", true}, rows[path], path)
			}
			gotest.Equal(it, 4, doc.Summary.NotRun)
			gotest.Equal(it, 3, doc.Summary.Classes["skip"])
			gotest.Equal(it, 4, doc.Summary.Classes["not-run"])
		})
	})
}
