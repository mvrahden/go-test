package audit_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditEvidenceTestSuite: saved streams and a coverage profile say what
// runs proved of the change a committed head makes to its base.
type AuditEvidenceTestSuite struct {
	dir, checkout, commit string
	baseFile, proof       string
	json, text, md        testkit.Run
}

func (s *AuditEvidenceTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *AuditEvidenceTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-evidence-")
	gotest.NoError(t, err)
	s.dir = dir
	s.baseFile = filepath.Join(dir, "base.json")
	gotest.Equal(t, 0, record(t, stage(t, dir, "base"), s.baseFile).Exit)
	s.checkout = stage(t, dir, "head")
	git(t, s.checkout, "-c", "init.defaultBranch=trunk", "init", "-q")
	git(t, s.checkout, "add", "-A")
	git(t, s.checkout, "-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-m", "head")
	s.commit = strings.TrimSpace(git(t, s.checkout, "rev-parse", "--short", "HEAD"))

	// The streams name the head's commit where they ran it.
	s.proof = filepath.Join(dir, "proof")
	gotest.NoError(t, os.MkdirAll(s.proof, 0o750))
	src := filepath.Join("..", "testdata", "audit", "proof")
	entries, err := os.ReadDir(src)
	gotest.NoError(t, err)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		gotest.NoError(t, err)
		gotest.NoError(t, os.WriteFile(filepath.Join(s.proof, e.Name()), []byte(strings.ReplaceAll(string(data), "HEAD_COMMIT", s.commit)), 0o600))
	}
	s.json = s.audit(t, "--format=json")
	s.text = s.audit(t)
	s.md = s.audit(t, "--format=md")
}

func (s *AuditEvidenceTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// audit compares the head with the base record, given the streams of four
// runs of the head, one of the base, a diff and a profile.
func (s *AuditEvidenceTestSuite) audit(t *gotest.T, args ...string) testkit.Run {
	p := func(name string) string { return filepath.Join(s.proof, name) }
	return s.cli(t, append([]string{
		"--evidence=" + p("another-commit.json"), "--evidence=" + p("current.json"),
		"--evidence=" + p("uncommitted.json"), "--evidence=" + p("no-manifest.json"),
		"--base-evidence=" + p("base-run.json"),
		"--diff=" + p("shop.diff"), "--coverage=" + p("shop.cover"),
	}, args...)...)
}

// cli runs audit in the checkout against the base record with args.
func (s *AuditEvidenceTestSuite) cli(t *gotest.T, args ...string) testkit.Run {
	run, err := testkit.RunCLIWithEnv(t.Context(), s.checkout, outside(s.dir), append([]string{"audit", "--base-record=" + s.baseFile}, args...)...)
	gotest.NoError(t, err)
	return run
}

// proofDoc is the part of an audit document the evidence suites read.
type proofDoc struct {
	Streams []struct {
		File string `json:"file"`
		Side string `json:"side"`
		Ran  bool   `json:"ran"`
		Run  *struct {
			Version  string `json:"version"`
			Commit   string `json:"commit"`
			Dirty    *bool  `json:"dirty"`
			Agent    string `json:"agent"`
			ExitCode *int   `json:"exitCode"`
		} `json:"run"`
		Current bool   `json:"current"`
		Stale   string `json:"stale"`
	} `json:"streams"`
	Evidence []evidence `json:"evidence"`
	Findings []struct {
		Class   string `json:"class"`
		Subject string `json:"subject"`
		Runtime *struct {
			Base *int `json:"base"`
			Head *int `json:"head"`
		} `json:"runtime"`
	} `json:"findings"`
	Unproven []unproven `json:"unproven"`
	Summary  struct {
		Unproven     int `json:"unproven"`
		ChangedLines int `json:"changedLines"`
		NoCoverage   int `json:"noCoverage"`
	} `json:"summary"`
}

type evidence struct {
	Path       []string `json:"path"`
	Verdict    string   `json:"verdict"`
	Assertions *int     `json:"assertions"`
	Vacuous    bool     `json:"vacuous"`
	AssertsBy  string   `json:"assertsBy"`
	SkipReason string   `json:"skipReason"`
	Stream     *int     `json:"stream"`
	Current    bool     `json:"current"`
	Others     []struct {
		Stream  int    `json:"stream"`
		Verdict string `json:"verdict"`
		Current bool   `json:"current"`
	} `json:"others"`
}

type unproven struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Coverage   bool   `json:"coverage"`
	Changed    int    `json:"changed"`
	Unexecuted int    `json:"unexecuted"`
	Ranges     []struct {
		Line    int `json:"line"`
		EndLine int `json:"endLine"`
	} `json:"ranges"`
}

func readProof(t *gotest.T, run testkit.Run) proofDoc { //nolint:gocritic // hugeParam: a run as RunCLI returns it
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	var doc proofDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	return doc
}

func (d *proofDoc) unprovenOf(file string) unproven {
	for _, u := range d.Unproven {
		if u.File == file {
			return u
		}
	}
	return unproven{}
}

func (d *proofDoc) evidenceOf(path string) evidence {
	for i := range d.Evidence {
		if strings.Join(d.Evidence[i].Path, " / ") == path {
			return d.Evidence[i]
		}
	}
	return evidence{}
}

func (s *AuditEvidenceTestSuite) TestEvidenceFromRuns(t *gotest.T) {
	t.When("audit is given a stream of the head's run", func(w *gotest.T) {
		w.It("states for each added, reworded, moved and changed behavior and each finding's behavior its verdict, its run-time assertions, whether it passed vacuous and its AssertsBy", func(it *gotest.T) {
			doc := readProof(it, s.json)
			type row struct {
				Path, Verdict string
				Assertions    int
				Vacuous       bool
				AssertsBy     string
			}
			var got []row
			for i := range doc.Evidence {
				e := &doc.Evidence[i]
				n := -1
				if e.Assertions != nil {
					n = *e.Assertions
				}
				got = append(got, row{strings.Join(e.Path, " / "), e.Verdict, n, e.Vacuous, e.AssertsBy})
			}
			gotest.Equal(it, []row{
				{"Cart / Copies / the only copy", "pass", 0, true, ""},
				{"Stock / Count / when the stock is empty", "not run", -1, false, ""},
				{"Stock / Count / when the stock is refilled", "pass", 1, false, ""},
				{"Cart / Add / when both numbers are positive / ignores the order", "pass", 1, false, ""},
				{"Stock / Count / when the stock is refilled / counts every item", "pass", 1, false, ""},
				{"Cart / Add / when one number is negative / returns the difference", "fail", 1, false, ""},
				{"Cart / Rounds", "pass", 1, false, "NotZero checks the sum"},
				{"Cart / Empty", "skip", -1, false, ""},
				{"Cart / Total", "pass", 1, false, ""},
				{"Stock", "pass", 4, false, ""},
				{"Cart", "fail", 6, true, ""},
			}, got)
		})
		w.It("names a behavior the stream did not report not run, one X_ excludes skipped, and one of a package no stream holds no evidence", func(it *gotest.T) {
			doc := readProof(it, s.json)
			gotest.Equal(it, "not run", doc.evidenceOf("Stock / Count / when the stock is empty").Verdict)
			empty := doc.evidenceOf("Cart / Empty")
			gotest.Equal(it, []string{"skip", "excluded by X_"}, []string{empty.Verdict, empty.SkipReason})
			other := readProof(it, s.cli(it, "--evidence="+filepath.Join(s.proof, "other-package.json"), "--format=json"))
			gotest.Len(it, other.Evidence, 11)
			for i := range other.Evidence {
				e := &other.Evidence[i]
				gotest.Equal(it, "no evidence", e.Verdict, "%v", e.Path)
				gotest.Zero(it, e.Stream, "%v", e.Path)
			}
		})
		w.It("lists the stream with its manifest's CLI version, commit, dirty flag, agent and exit code", func(it *gotest.T) {
			doc := readProof(it, s.json)
			gotest.Len(it, doc.Streams, 5)
			st := doc.Streams[1]
			gotest.Equal(it, []string{filepath.Join(s.proof, "current.json"), "head"}, []string{st.File, st.Side})
			gotest.NotZero(it, st.Run)
			gotest.Equal(it, []string{"v1.42.0", s.commit, "claude-code"}, []string{st.Run.Version, st.Run.Commit, st.Run.Agent})
			gotest.False(it, *st.Run.Dirty)
			gotest.Equal(it, 1, *st.Run.ExitCode)
			gotest.True(it, st.Current)
			gotest.Equal(it, "base", doc.Streams[4].Side)
		})
	})
}

func (s *AuditEvidenceTestSuite) TestCurrentAndStaleEvidence(t *gotest.T) {
	t.When("audit is given streams of another commit, of a tree with uncommitted changes and without a manifest", func(w *gotest.T) {
		w.It("marks each stale, naming another commit, uncommitted changes and no manifest", func(it *gotest.T) {
			doc := readProof(it, s.json)
			var got []string
			for _, st := range doc.Streams[:4] {
				got = append(got, st.Stale)
				if st.Stale != "" {
					gotest.False(it, st.Current, st.File)
				}
			}
			gotest.Equal(it, []string{
				"another commit: the stream ran 0a1b2c3, the head is " + s.commit,
				"",
				"the stream's tree had uncommitted changes",
				"the stream has no manifest",
			}, got)
			gotest.Zero(it, doc.Streams[3].Run)
		})
	})
	t.When("a current and a stale stream judge one behavior", func(w *gotest.T) {
		w.It("states the current stream's verdict and names the stale one's among the others", func(it *gotest.T) {
			doc := readProof(it, s.json)
			e := doc.evidenceOf("Cart / Add / when one number is negative / returns the difference")
			gotest.Equal(it, "fail", e.Verdict)
			gotest.NotZero(it, e.Stream)
			gotest.Equal(it, 1, *e.Stream)
			gotest.True(it, e.Current)
			type other struct {
				Stream  int
				Verdict string
				Current bool
			}
			var got []other
			for _, o := range e.Others {
				got = append(got, other{o.Stream, o.Verdict, o.Current})
			}
			gotest.Equal(it, []other{{0, "pass", false}, {2, "pass", false}, {3, "pass", false}}, got)
		})
	})
}

func (s *AuditEvidenceTestSuite) TestAssertionsAtRunTime(t *gotest.T) {
	t.When("audit is given a stream of each side's run and the head drops an assertion", func(w *gotest.T) {
		w.It("the assertions finding carries the base's and the head's run-time counts", func(it *gotest.T) {
			doc := readProof(it, s.json)
			var got []int
			for i := range doc.Findings {
				f := &doc.Findings[i]
				if f.Class == "assertions" {
					gotest.Equal(it, "Cart / Rounds", f.Subject)
					gotest.NotZero(it, f.Runtime)
					gotest.NotZero(it, f.Runtime.Base)
					gotest.NotZero(it, f.Runtime.Head)
					got = append(got, *f.Runtime.Base, *f.Runtime.Head)
				}
			}
			gotest.Equal(it, []int{4, 1}, got)
			gotest.Contains(it, s.text.Stdout, "Cart / Rounds: 2 → 1 (run time 4 → 1) (shop/cart_suite_test.go:43)\n")
		})
	})
}

func (s *AuditEvidenceTestSuite) TestUnprovenLines(t *gotest.T) {
	t.When("audit is given a coverage profile and a diff that changes production lines", func(w *gotest.T) {
		w.It("lists per file the changed executable lines no test executed as ranges, and counts them in the summary", func(it *gotest.T) {
			doc := readProof(it, s.json)
			gotest.Len(it, doc.Unproven, 2)
			shop := doc.unprovenOf("shop/shop.go")
			gotest.Equal(it, []int{10, 3}, []int{shop.Line, shop.Unexecuted})
			var ranges [][2]int
			for _, r := range shop.Ranges {
				ranges = append(ranges, [2]int{r.Line, r.EndLine})
			}
			gotest.Equal(it, [][2]int{{10, 11}, {15, 15}}, ranges)
			gotest.Equal(it, 3, doc.Summary.Unproven)
			gotest.Contains(it, s.text.Stdout, "\nunproven: 3 of 7 changed executable lines not executed, 1 file without coverage data\n")
		})
		w.It("counts no changed line that no block of the profile spans", func(it *gotest.T) {
			doc := readProof(it, s.json)
			// The diff adds 11 lines to shop.go: a blank, a comment, the
			// signature and the closing brace are in no block.
			gotest.Equal(it, 7, doc.unprovenOf("shop/shop.go").Changed)
			gotest.Equal(it, 7, doc.Summary.ChangedLines)
		})
		w.It("lists a changed file of a package the profile does not cover as without coverage data", func(it *gotest.T) {
			doc := readProof(it, s.json)
			ledger := doc.unprovenOf("shop/ledger/ledger.go")
			gotest.Equal(it, "shop/ledger/ledger.go", ledger.File)
			gotest.False(it, ledger.Coverage)
			gotest.Equal(it, []int{0, 0}, []int{ledger.Changed, ledger.Unexecuted})
			gotest.Equal(it, 1, doc.Summary.NoCoverage)
			gotest.Contains(it, s.text.Stdout, "\n  shop/ledger/ledger.go  no coverage data\n")
		})
	})
}

func (s *AuditEvidenceTestSuite) TestProofInThePolicy(t *gotest.T) {
	t.When("--fail-on names failing and a stream fails a behavior the comparison names", func(w *gotest.T) {
		w.It("exits 1 and the closing line counts the evidence and names the class", func(it *gotest.T) {
			run := s.audit(it, "--fail-on=failing")
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `\ngotest audit: FAIL exit 1 \| weakenings: 13 \(.*\) \| behaviors: .* \| evidence: 11 \(failing 2, not run 2, vacuous 2, stale 0\) \| unproven: 3 of 7 lines, 1 without coverage \| fail-on: failing\n$`, run.Stdout)
			gotest.Equal(it, 0, s.text.Exit, "without the policy")
		})
		w.It("--fail-on=all names every weakening class and no proof class", func(it *gotest.T) {
			run := s.audit(it, "--fail-on=all")
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `\| fail-on: removed,excluded,focus,guard,budget,suppression,snapshot,assertions,asserts-by\n$`, run.Stdout)
		})
	})
	t.When("--fail-on names a proof class and no stream or profile is given", func(w *gotest.T) {
		w.It("audit refuses it with exit 2, naming the flags that give it", func(it *gotest.T) {
			for flag, want := range map[string]string{
				"stale":    "FAIL: --fail-on=stale judges runs: give --evidence=<stream> or --run\n",
				"unproven": "FAIL: --fail-on=unproven judges a coverage profile: give --coverage or --run, with --base or --diff\n",
			} {
				run := s.cli(it, "--fail-on="+flag)
				gotest.Equal(it, 2, run.Exit, flag)
				gotest.Empty(it, run.Stdout, flag)
				gotest.Equal(it, want, run.Stderr)
			}
		})
	})
}

func (s *AuditEvidenceTestSuite) TestEvidenceInTheRenderings(t *gotest.T) {
	t.When("audit renders evidence in the text format", func(w *gotest.T) {
		w.It("marks each row with its verdict and names each stream read under the first line", func(it *gotest.T) {
			gotest.Equal(it, 0, s.text.Exit, "stderr: %s", s.text.Stderr)
			gotest.Empty(it, s.text.Stderr)
			p := func(name string) string { return filepath.Join(s.proof, name) }
			lines := strings.Split(s.text.Stdout, "\n")
			gotest.Equal(it, []string{
				"evidence: 5 streams, 1 current; * marks a verdict of a stale one",
				"  stale    " + p("another-commit.json") + " of 0a1b2c3, exit 0: another commit: the stream ran 0a1b2c3, the head is " + s.commit,
				"  current  " + p("current.json") + " of " + s.commit + ", exit 1",
				"  stale    " + p("uncommitted.json") + " of " + s.commit + ", exit 0: the stream's tree had uncommitted changes",
				"  stale    " + p("no-manifest.json") + ": the stream has no manifest",
				"  stale    base " + p("base-run.json") + " of 9f8e7d6, exit 0: the base names no commit",
			}, lines[1:7])
			gotest.Contains(it, s.text.Stdout, "\n  changed    FAIL         ./shop Cart / Add / when one number is negative / returns the difference (shop/cart_suite_test.go:29)\n")
			gotest.Contains(it, s.text.Stdout, "\n  added      vacuous      ./shop Cart / Copies / the only copy (shop/cart_suite_test.go:49)\n")
			gotest.Contains(it, s.text.Stdout, "\n  focus        pass 1       ./shop Cart / Total: TestTotal → F_TestTotal (shop/cart_suite_test.go:39)\n")
		})
		w.It("lists the removed, weakened and failing behaviors before the others", func(it *gotest.T) {
			var rows []string
			in := false
			for _, line := range strings.Split(s.text.Stdout, "\n") {
				switch {
				case strings.HasPrefix(line, "behaviors: "):
					in = true
				case in && line == "":
					in = false
				case in:
					f := strings.Fields(line)
					rows = append(rows, f[0]+" "+f[len(f)-1])
				}
			}
			gotest.Equal(it, []string{
				"removed (shop/cart_suite_test.go:50)",
				"removed (shop/cart_suite_test.go:53)",
				"removed (shop/cart_suite_test.go:32)",
				"changed (shop/cart_suite_test.go:43)",
				"changed (shop/cart_suite_test.go:29)",
				"added (shop/cart_suite_test.go:49)",
				"added (shop/stock_suite_test.go:30)",
				"added (shop/stock_suite_test.go:25)",
				"reworded (shop/cart_suite_test.go:21)",
				"moved shop/stock_suite_test.go:26)",
			}, rows)
		})
	})
	t.When("the text format lists five or more rows alike of one suite", func(w *gotest.T) {
		w.It("folds them into one line that counts them, and -v lists each", func(it *gotest.T) {
			// Every leaf of the Cart suite gets a body the head does not have.
			headFile := filepath.Join(s.dir, "head.json")
			gotest.Equal(it, 0, record(it, s.checkout, headFile).Exit)
			data, err := os.ReadFile(headFile)
			gotest.NoError(it, err)
			var rec map[string]any
			gotest.NoError(it, json.Unmarshal(data, &rec))
			for _, p := range rec["packages"].([]any) {
				for _, suite := range p.(map[string]any)["suites"].([]any) {
					if suite.(map[string]any)["name"] == "CartTestSuite" {
						rehashLeaves(suite.(map[string]any)["methods"].([]any))
					}
				}
			}
			out, err := json.Marshal(rec)
			gotest.NoError(it, err)
			changed := filepath.Join(s.dir, "rehashed.json")
			gotest.NoError(it, os.WriteFile(changed, out, 0o600))
			args := []string{"audit", "--base-record=" + changed, "--head-record=" + headFile}
			folded, err := testkit.RunCLI(it.Context(), s.dir, args...)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, folded.Exit, "stderr: %s", folded.Stderr)
			gotest.Contains(it, folded.Stdout, "\nbehaviors: 0 added, 0 removed, 0 reworded, 0 moved, 7 changed\n  changed    ./shop Cart: 7 behaviors\n")
			gotest.Contains(it, folded.Stdout, "\n  7 rows alike are grouped by package and suite; -v lists every one\n")
			each, err := testkit.RunCLI(it.Context(), s.dir, append(args, "-v")...)
			gotest.NoError(it, err)
			gotest.Equal(it, 7, strings.Count(each.Stdout, "\n  changed    ./shop Cart / "), each.Stdout)
			gotest.NotContains(it, each.Stdout, "grouped")
		})
	})
	t.When("audit renders evidence with --format=md", func(w *gotest.T) {
		w.It("writes the evidence as a column of the behaviors table and the unproven lines as a table", func(it *gotest.T) {
			gotest.Equal(it, 0, s.md.Exit, "stderr: %s", s.md.Stderr)
			gotest.Contains(it, s.md.Stdout, "\n#### Behaviors\n\n| Change | Evidence | Package | Behavior | Where |\n|---|---|---|---|---|\n| removed | — | ./shop | Cart / Copies / first copy | shop/cart\\_suite\\_test.go:50 |\n")
			gotest.Contains(it, s.md.Stdout, "\n| changed | FAIL | ./shop | Cart / Add / when one number is negative / returns the difference | shop/cart\\_suite\\_test.go:29 |\n")
			gotest.Contains(it, s.md.Stdout, "\n#### Unproven lines\n\n3 of 7 changed executable lines not executed, 1 file without coverage data\n\n| File | Package | Not executed |\n|---|---|---|\n| shop/ledger/ledger.go | ./shop/ledger | no coverage data |\n| shop/shop.go | ./shop | 3 of 7: 10-11, 15 |\n")
		})
	})
}

// rehashLeaves gives every leaf among bs another body hash.
func rehashLeaves(bs []any) {
	for _, b := range bs {
		m := b.(map[string]any)
		kids, _ := m["children"].([]any)
		if len(kids) == 0 {
			m["hash"] = "0000000000000000"
			continue
		}
		rehashLeaves(kids)
	}
}

// toolDoc is the document as a tool reads it, field by field.
type toolDoc struct {
	Head       struct{ Branch, Commit string } `json:"head"`
	Base       struct{ Branch, Commit string } `json:"base"`
	ModuleDirs []struct{ Path, Dir string }    `json:"moduleDirs"`
	Findings   []struct {
		ID, Class, Module, Package, Subject string
		BaseLocation                        *struct{ File string } `json:"baseLocation"`
		HeadLocation                        *struct{ File string } `json:"headLocation"`
		Test, Run                           string
	} `json:"findings"`
	Behaviors []struct {
		Change    string
		Head      *struct{ Path []string } `json:"head"`
		Test, Run string
	} `json:"behaviors"`
	Evidence []struct {
		ID             string
		Path           []string
		Verdict        string
		NeedsDecision  bool `json:"needsDecision"`
		Source, Commit string
		Test, Run      string
	} `json:"evidence"`
}

func readTool(t *gotest.T, run testkit.Run) toolDoc { //nolint:gocritic // hugeParam: a run as RunCLI returns it
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	var doc toolDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	return doc
}

func (s *AuditEvidenceTestSuite) TestWhatAToolReads(t *gotest.T) {
	t.When("a tool reads the audit document as JSON", func(w *gotest.T) {
		w.It("finds on each finding and evidence row an id that stays when the lines move", func(it *gotest.T) {
			// The head's record with every line moved down by 100.
			headFile := filepath.Join(s.dir, "moved-head.json")
			gotest.Equal(it, 0, record(it, s.checkout, headFile).Exit)
			data, err := os.ReadFile(headFile)
			gotest.NoError(it, err)
			var rec any
			gotest.NoError(it, json.Unmarshal(data, &rec))
			moveLines(rec)
			out, err := json.Marshal(rec)
			gotest.NoError(it, err)
			gotest.NoError(it, os.WriteFile(headFile, out, 0o600))
			moved := readTool(it, s.cli(it, "--head-record="+headFile, "--evidence="+filepath.Join(s.proof, "current.json"), "--format=json"))
			doc := readTool(it, s.json)
			ids := func(d *toolDoc) []string {
				var out []string
				for i := range d.Findings {
					gotest.Regexp(it, `^[0-9a-f]{16}$`, d.Findings[i].ID)
					out = append(out, d.Findings[i].ID)
				}
				for i := range d.Evidence {
					gotest.Regexp(it, `^[0-9a-f]{16}$`, d.Evidence[i].ID)
					out = append(out, d.Evidence[i].ID)
				}
				return out
			}
			want := ids(&doc)
			gotest.Len(it, want, 16+11)
			gotest.Equal(it, want, ids(&moved))
			seen := map[string]bool{}
			for _, id := range want {
				gotest.False(it, seen[id], "id %s twice", id)
				seen[id] = true
			}
		})
		w.It("finds on a finding each side's location, and the module of one without a package", func(it *gotest.T) {
			doc := readTool(it, s.json)
			got := map[string][2]string{}
			for i := range doc.Findings {
				f := &doc.Findings[i]
				var b, h string
				if f.BaseLocation != nil {
					b = f.BaseLocation.File
				}
				if f.HeadLocation != nil {
					h = f.HeadLocation.File
				}
				got[f.Class+" "+f.Subject] = [2]string{b, h}
				gotest.Equal(it, "auditmod", f.Module, f.Subject)
			}
			const cart, stock = "shop/cart_suite_test.go", "shop/stock_suite_test.go"
			gotest.Equal(it, [2]string{cart, ""}, got["removed Cart / Remove"])
			gotest.Equal(it, [2]string{cart, cart}, got["focus Cart / Total"])
			gotest.Equal(it, [2]string{"", stock}, got["suppression "+stock])
			gotest.Equal(it, [2]string{"shop/plain_test.go", "shop/plain_test.go"}, got["suppression shop/plain_test.go"])
			gone := "shop/testdata/__snapshots__/TestStockTestSuite_ext.snap"
			gotest.Equal(it, [2]string{gone, ""}, got["snapshot "+gone])
		})
		w.It("finds on a finding, behavior and evidence row about a test the go test name and -run pattern that select it", func(it *gotest.T) {
			doc := readTool(it, s.json)
			const test = "TestCartTestSuite/TestAdd/one_number_is_negative/returns_the_difference"
			const run = "^TestCartTestSuite$/^TestAdd$/^one_number_is_negative$/^returns_the_difference$"
			found := 0
			for i := range doc.Findings {
				f := &doc.Findings[i]
				if f.Class == "changed" && f.Subject == "Cart / Add / when one number is negative / returns the difference" {
					gotest.Equal(it, []string{test, run, "auditmod/shop"}, []string{f.Test, f.Run, f.Package})
					found++
				}
				if f.Class == "removed" && f.Subject == "Cart / Remove" {
					gotest.Equal(it, []string{"TestCartTestSuite/TestRemove", "^TestCartTestSuite$/^TestRemove$"}, []string{f.Test, f.Run})
					found++
				}
				if f.Package == "" {
					gotest.Empty(it, f.Test, f.Subject)
				}
			}
			for i := range doc.Behaviors {
				c := &doc.Behaviors[i]
				if c.Change == "changed" && c.Head.Path[1] == "Add" {
					gotest.Equal(it, []string{test, run}, []string{c.Test, c.Run})
					found++
				}
			}
			for i := range doc.Evidence {
				e := &doc.Evidence[i]
				if strings.Join(e.Path, " / ") == "Cart / Total" {
					gotest.Equal(it, []string{"TestCartTestSuite/F_TestTotal", "^TestCartTestSuite$/^F_TestTotal$"}, []string{e.Test, e.Run})
					found++
				}
			}
			gotest.Equal(it, 4, found)
		})
		w.It("finds each module's directory relative to the audited one and each side's branch", func(it *gotest.T) {
			doc := readTool(it, s.json)
			gotest.Len(it, doc.ModuleDirs, 1)
			gotest.Equal(it, []string{"auditmod", "."}, []string{doc.ModuleDirs[0].Path, doc.ModuleDirs[0].Dir})
			gotest.Equal(it, []string{"trunk", s.commit}, []string{doc.Head.Branch, doc.Head.Commit})
			gotest.Empty(it, doc.Base.Branch, "a record of no checkout")
			viaGit := readTool(it, s.cli2(it, "audit", "--base=trunk", "--format=json"))
			gotest.Equal(it, "trunk", viaGit.Base.Branch)
		})
		w.It("finds on each evidence row whether it needs a decision, the stream it was judged by and that stream's commit", func(it *gotest.T) {
			doc := readTool(it, s.json)
			got := map[string]bool{}
			for i := range doc.Evidence {
				e := &doc.Evidence[i]
				got[strings.Join(e.Path, " / ")] = e.NeedsDecision
				gotest.Equal(it, []string{filepath.Join(s.proof, "current.json"), s.commit}, []string{e.Source, e.Commit}, "%v", e.Path)
			}
			gotest.Equal(it, map[string]bool{
				"Cart / Copies / the only copy":                                     true,
				"Stock / Count / when the stock is empty":                           true,
				"Stock / Count / when the stock is refilled":                        false,
				"Cart / Add / when both numbers are positive / ignores the order":   false,
				"Stock / Count / when the stock is refilled / counts every item":    false,
				"Cart / Add / when one number is negative / returns the difference": true,
				"Cart / Rounds": false,
				"Cart / Empty":  false,
				"Cart / Total":  false,
				"Stock":         true,
				"Cart":          true,
			}, got)
			stale := readTool(it, s.cli(it, "--evidence="+filepath.Join(s.proof, "another-commit.json"), "--format=json"))
			for i := range stale.Evidence {
				e := &stale.Evidence[i]
				gotest.True(it, e.NeedsDecision, "%v judged by a stale stream", e.Path)
				gotest.Equal(it, "0a1b2c3", e.Commit)
			}
		})
	})
}

// cli2 runs the CLI in the checkout with args alone.
func (s *AuditEvidenceTestSuite) cli2(t *gotest.T, args ...string) testkit.Run {
	run, err := testkit.RunCLIWithEnv(t.Context(), s.checkout, outside(s.dir), args...)
	gotest.NoError(t, err)
	return run
}

// moveLines adds 100 to every line a record names.
func moveLines(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if n, ok := val.(float64); ok && (k == "line" || k == "endLine") {
				x[k] = n + 100
				continue
			}
			moveLines(val)
		}
	case []any:
		for _, val := range x {
			moveLines(val)
		}
	}
}

// linkDoc is the document as a tool links its rows.
type linkDoc struct {
	Findings []struct {
		Class, Package, Subject, Evidence string
	} `json:"findings"`
	Behaviors []struct {
		Change, Module, Evidence string
		Order                    int
		Head                     *struct {
			Path []string
			File string
			Line int
		} `json:"head"`
	} `json:"behaviors"`
	Evidence []struct {
		ID, Package, Module, Mark string
		Path                      []string
		Location                  *struct {
			File          string
			Line, EndLine int
		} `json:"location"`
	} `json:"evidence"`
}

func readLinks(t *gotest.T, run testkit.Run) linkDoc { //nolint:gocritic // hugeParam: a run as RunCLI returns it
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	var doc linkDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
	return doc
}

// idOf is the id of the evidence row about path, "" for none.
func (d *linkDoc) idOf(path string) string {
	for i := range d.Evidence {
		if strings.Join(d.Evidence[i].Path, " / ") == path {
			return d.Evidence[i].ID
		}
	}
	return ""
}

// textRows are the rows of one list of the text format, from its title to
// the blank line after it.
func textRows(stdout, title string) []string {
	var rows []string
	in := false
	for _, line := range strings.Split(stdout, "\n") {
		switch {
		case strings.HasPrefix(line, title):
			in = true
		case in && line == "":
			return rows
		case in:
			rows = append(rows, line)
		}
	}
	return rows
}

func (s *AuditEvidenceTestSuite) TestRowsAToolLinks(t *gotest.T) {
	t.When("a tool reads the audit document with evidence as JSON", func(w *gotest.T) {
		w.It("finds on each finding and behavior about an evidence row's behavior that row's id, and none on the others", func(it *gotest.T) {
			doc := readLinks(it, s.json)
			linked := 0
			for _, f := range doc.Findings {
				want := ""
				if f.Package != "" && f.Class != "removed" {
					want = doc.idOf(f.Subject)
					gotest.NotEmpty(it, want, f.Subject)
					linked++
				}
				gotest.Equal(it, want, f.Evidence, "%s %s", f.Class, f.Subject)
			}
			gotest.Equal(it, 9, linked)
			for _, c := range doc.Behaviors {
				if c.Head == nil {
					gotest.Empty(it, c.Evidence, c.Change)
					continue
				}
				gotest.Equal(it, doc.idOf(strings.Join(c.Head.Path, " / ")), c.Evidence, "%v", c.Head.Path)
				gotest.NotEmpty(it, c.Evidence, "%v", c.Head.Path)
			}
		})
		w.It("finds on each behavior and evidence row its module, and on each evidence row the head's file and lines of its behavior", func(it *gotest.T) {
			doc := readLinks(it, s.json)
			for _, c := range doc.Behaviors {
				gotest.Equal(it, "auditmod", c.Module, c.Change)
			}
			type loc struct {
				File          string
				Line, EndLine int
			}
			got := map[string]loc{}
			for _, e := range doc.Evidence {
				gotest.Equal(it, "auditmod", e.Module, "%v", e.Path)
				gotest.NotZero(it, e.Location, "%v", e.Path)
				got[strings.Join(e.Path, " / ")] = loc{e.Location.File, e.Location.Line, e.Location.EndLine}
			}
			const cart, stock = "shop/cart_suite_test.go", "shop/stock_suite_test.go"
			gotest.Equal(it, map[string]loc{
				"Cart / Copies / the only copy":                                     {cart, 49, 51},
				"Stock / Count / when the stock is empty":                           {stock, 30, 34},
				"Stock / Count / when the stock is refilled":                        {stock, 25, 29},
				"Cart / Add / when both numbers are positive / ignores the order":   {cart, 21, 26},
				"Stock / Count / when the stock is refilled / counts every item":    {stock, 26, 28},
				"Cart / Add / when one number is negative / returns the difference": {cart, 29, 31},
				"Cart / Rounds": {cart, 43, 46},
				"Cart / Empty":  {cart, 35, 37},
				"Cart / Total":  {cart, 39, 41},
				"Stock":         {stock, 11, 11},
				"Cart":          {cart, 9, 9},
			}, got)
		})
		w.It("finds on each evidence row the mark the text format writes on its rows", func(it *gotest.T) {
			doc := readLinks(it, s.json)
			got := map[string]string{}
			for _, e := range doc.Evidence {
				got[strings.Join(e.Path, " / ")] = e.Mark
			}
			gotest.Equal(it, map[string]string{
				"Cart / Copies / the only copy":                                     "vacuous",
				"Stock / Count / when the stock is empty":                           "not run",
				"Stock / Count / when the stock is refilled":                        "pass",
				"Cart / Add / when both numbers are positive / ignores the order":   "pass 1",
				"Stock / Count / when the stock is refilled / counts every item":    "pass 1",
				"Cart / Add / when one number is negative / returns the difference": "FAIL",
				"Cart / Rounds": "pass 1",
				"Cart / Empty":  "skip",
				"Cart / Total":  "pass 1",
				"Stock":         "partly run",
				"Cart":          "FAIL",
			}, got)
			gotest.Contains(it, s.text.Stdout, "\n  guard        partly run   ./shop Stock: no SuiteGuard → SuiteGuard (shop/stock_suite_test.go:11)\n")
			stale := readLinks(it, s.cli(it, "--evidence="+filepath.Join(s.proof, "another-commit.json"), "--format=json"))
			staleText := s.cli(it, "--evidence="+filepath.Join(s.proof, "another-commit.json"))
			gotest.Len(it, stale.Evidence, 11)
			for _, e := range stale.Evidence {
				gotest.Regexp(it, `\*$`, e.Mark, "%v", e.Path)
				if strings.Join(e.Path, " / ") == "Cart / Total" {
					gotest.Equal(it, "pass 1*", e.Mark)
					gotest.Contains(it, staleText.Stdout, "\n  focus        pass 1*      ./shop Cart / Total: TestTotal → F_TestTotal (shop/cart_suite_test.go:39)\n")
				}
			}
		})
		w.It("finds on each behavior its place in the text format's list of behaviors", func(it *gotest.T) {
			doc := readLinks(it, s.json)
			marks := map[string]string{}
			for _, e := range doc.Evidence {
				marks[e.ID] = e.Mark
			}
			rows := textRows(s.text.Stdout, "behaviors: ")
			gotest.Len(it, rows, 10)
			gotest.Len(it, doc.Behaviors, 10)
			seen := map[int]bool{}
			for _, c := range doc.Behaviors {
				gotest.False(it, seen[c.Order], "order %d twice", c.Order)
				seen[c.Order] = true
				gotest.True(it, c.Order >= 0 && c.Order < len(rows), "order %d", c.Order)
				f := strings.Fields(rows[c.Order])
				gotest.Equal(it, c.Change, f[0], rows[c.Order])
				if c.Head != nil {
					gotest.Contains(it, rows[c.Order], " "+marks[c.Evidence]+" ")
					gotest.Contains(it, rows[c.Order], c.Head.Path[len(c.Head.Path)-1])
					gotest.Contains(it, rows[c.Order], fmt.Sprintf("%s:%d)", c.Head.File, c.Head.Line))
				}
			}
			byOrder := make([]string, len(doc.Behaviors))
			for _, c := range doc.Behaviors {
				byOrder[c.Order] = c.Change
			}
			gotest.Equal(it, []string{"removed", "removed", "removed", "changed", "changed", "added", "added", "added", "reworded", "moved"}, byOrder)
		})
	})
}

func (s *AuditEvidenceTestSuite) TestAStoppedAuditRun(t *gotest.T) {
	t.When("audit --run --control=stdin has its standard input at its end", func(w *gotest.T) {
		w.It("writes the document naming the stopped run among what was not compared and exits 130", func(it *gotest.T) {
			run := s.cli(it, "--run", "--control=stdin", "--format=json")
			gotest.Equal(it, 130, run.Exit, "stderr: %s", run.Stderr)
			var doc struct {
				Incomplete []string `json:"incomplete"`
				Streams    []struct {
					Ran bool
					Run *struct{ ExitCode *int } `json:"run"`
				} `json:"streams"`
				Evidence []struct{ Verdict string } `json:"evidence"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &doc), "document: %s", run.Stdout)
			gotest.Equal(it, []string{"--run: the run was stopped before its end; what it did not reach reads as not run"}, doc.Incomplete)
			gotest.Len(it, doc.Streams, 1)
			gotest.True(it, doc.Streams[0].Ran)
			gotest.NotZero(it, doc.Streams[0].Run)
			gotest.Equal(it, 130, *doc.Streams[0].Run.ExitCode)
			gotest.Len(it, doc.Evidence, 11)
		})
		w.It("the text format's closing line names exit 130", func(it *gotest.T) {
			run := s.cli(it, "--run", "--control=stdin", "--fail-on=all")
			gotest.Equal(it, 130, run.Exit, "stderr: %s", run.Stderr)
			gotest.Contains(it, run.Stdout, "\nnot compared:\n  --run: the run was stopped before its end; what it did not reach reads as not run\n")
			gotest.Regexp(it, `\ngotest audit: FAIL exit 130 \| .* \| not compared: 1 \| fail-on: removed,`, run.Stdout)
		})
	})
	t.When("--control=stdin is given to audit without --run, or beside --evidence=-", func(w *gotest.T) {
		w.It("audit refuses it with exit 2, naming --run or standard input", func(it *gotest.T) {
			type refusal struct {
				args []string
				want string
			}
			for _, row := range []refusal{
				{[]string{"--control=stdin"}, "FAIL: --control=stdin stops the run --run makes; give it with --run\n"},
				{[]string{"--run", "--control=stdin", "--evidence=-"}, "FAIL: --control=stdin and a stream or diff from - both read standard input; save it to a file\n"},
			} {
				run := s.cli(it, row.args...)
				gotest.Equal(it, 2, run.Exit, "%v", row.args)
				gotest.Empty(it, run.Stdout, "%v", row.args)
				gotest.Equal(it, row.want, run.Stderr)
			}
		})
	})
}
