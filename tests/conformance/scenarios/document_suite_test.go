package scenarios_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScenariosDocumentTestSuite: one document, written as JSON, text and
// Markdown, and the exit code its policy gives.
type ScenariosDocumentTestSuite struct {
	dir, mod, events          string
	json, text, md            testkit.Run
	failing, onUnbound        testkit.Run
	unknown, needsRun, broken testkit.Run
}

func (s *ScenariosDocumentTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *ScenariosDocumentTestSuite) BeforeAll(t *gotest.T) {
	s.dir = tempDir(t, "gotest-scenarios-document-")
	s.mod = stage(t, s.dir, "mod")
	s.json = scenarios(t, s.mod, "--format=json")
	s.text = scenarios(t, s.mod)
	s.md = scenarios(t, s.mod, "--format=md")

	events, err := testkit.RunCLI(t.Context(), s.mod, "-json", "./checkout/...")
	gotest.NoError(t, err)
	s.events = filepath.Join(s.dir, "events.json")
	gotest.NoError(t, os.WriteFile(s.events, []byte(events.Stdout), 0o600))
	s.failing = scenarios(t, s.mod, "--input="+s.events)
	s.onUnbound = scenarios(t, s.mod, "--fail-on=unbound")
	s.unknown = scenarios(t, s.mod, "--fail-on=unbound,flaky")
	s.needsRun = scenarios(t, s.mod, "--fail-on=failing")

	broken := stage(t, s.dir, "broken")
	pkg := filepath.Join(broken, "broken")
	gotest.NoError(t, os.MkdirAll(pkg, 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(pkg, "broken.go"), []byte("package broken\n\nfunc Two() int { return \"two\" }\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(pkg, "broken_test.go"), []byte("package broken\n"), 0o600))
	s.broken = scenarios(t, broken)
}

func (s *ScenariosDocumentTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *ScenariosDocumentTestSuite) TestTheScenariosDocument(t *gotest.T) {
	t.When("scenarios writes --format=json", func(w *gotest.T) {
		w.It("prints one document opening with version and schema that holds evidence, run, specs, summary and scenarios, each with its clauses and their Its, with no absolute path", func(it *gotest.T) {
			gotest.Equal(it, 0, s.json.Exit, "stderr: %s", s.json.Stderr)
			gotest.Regexp(it, `^\{\n  "version": "[^"]+",\n  "schema": 1,\n  "evidence": "none",`, s.json.Stdout)
			for _, key := range []string{`"specs":`, `"summary":`, `"scenarios":`, `"clauses":`, `"its":`} {
				gotest.Contains(it, s.json.Stdout, key)
			}
			gotest.NotContains(it, s.json.Stdout, s.dir)
			doc := readDoc(it, &s.json)
			gotest.Equal(it, 18, doc.Summary.Scenarios)
			gotest.Equal(it, map[string]int{"bound": 11, "unbound": 4, "ambiguous": 3}, doc.Summary.Statuses)
			first := doc.Scenarios[0]
			gotest.Equal(it, "openspec/specs/stock/spec.md:11", first.File+":"+strconv.Itoa(first.Line))
		})
	})

	t.When("scenarios writes the text format", func(w *gotest.T) {
		w.It("lists the scenarios that are not bound or verified, each with the clause at fault, and ends stdout with a closing line naming the verdict, the exit code and the counts, with nothing on stderr", func(it *gotest.T) {
			gotest.Equal(it, 0, s.text.Exit)
			gotest.Empty(it, s.text.Stderr)
			gotest.Contains(it, s.text.Stdout, "unbound    specs/cart/spec.md:25  Totals / A coupon\n             AND the coupon is used up: no It under the When says it\n")
			gotest.Contains(it, s.text.Stdout, "ambiguous  specs/cart/spec.md:49  Ambiguity / Stock counted twice\n             THEN counts each item: 2 Its say it: shop/stock_suite_test.go:17, shop/stock_suite_test.go:25\n")
			gotest.NotContains(it, s.text.Stdout, "An empty cart")
			lines := strings.Split(strings.TrimSuffix(s.text.Stdout, "\n"), "\n")
			gotest.Equal(it, "gotest scenarios: PASS exit 0 | scenarios: 18 (11 bound, 4 unbound, 3 ambiguous) | evidence: none", lines[len(lines)-1])
		})
	})

	t.When("scenarios writes --format=md", func(w *gotest.T) {
		w.It("writes a heading and a table of requirements and scenarios per capability", func(it *gotest.T) {
			gotest.Regexp(it, `^## Scenarios\n`, s.md.Stdout)
			for _, capability := range []string{"cart", "checkout", "stock"} {
				gotest.Contains(it, s.md.Stdout, "\n### "+capability+"\n")
			}
			gotest.Equal(it, 3, strings.Count(s.md.Stdout, "| Requirement | Scenario | Status | Its |\n|---|---|---|---|\n"))
			gotest.Contains(it, s.md.Stdout, "| Totals | An empty cart | bound | `shop/cart_suite_test.go:17`, `shop/cart_suite_test.go:20` |\n")
		})
	})
}

func (s *ScenariosDocumentTestSuite) TestThePolicyExit(t *gotest.T) {
	t.When("scenarios finds unbound scenarios without evidence and without --fail-on", func(w *gotest.T) {
		w.It("exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, s.text.Exit)
			gotest.Contains(it, s.text.Stdout, "4 unbound")
		})
	})

	t.When("the evidence holds a failing scenario and no --fail-on is given", func(w *gotest.T) {
		w.It("exits 1 and the closing line names FAIL and the statuses failing and ambiguous", func(it *gotest.T) {
			gotest.Equal(it, 1, s.failing.Exit, "stderr: %s", s.failing.Stderr)
			lines := strings.Split(strings.TrimSuffix(s.failing.Stdout, "\n"), "\n")
			gotest.Regexp(it, `^gotest scenarios: FAIL exit 1 \| .*2 failing.* \| evidence: input \| fail-on: ambiguous,failing$`, lines[len(lines)-1])
		})
	})

	t.When("--fail-on=unbound is given and a scenario is unbound", func(w *gotest.T) {
		w.It("exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, s.onUnbound.Exit, "stderr: %s", s.onUnbound.Stderr)
		})
	})

	t.When("--fail-on names an unknown status", func(w *gotest.T) {
		w.It("refuses it with exit 2, naming the statuses", func(it *gotest.T) {
			gotest.Equal(it, 2, s.unknown.Exit)
			gotest.Empty(it, s.unknown.Stdout)
			gotest.Contains(it, s.unknown.Stderr, `unknown status "flaky"; the statuses are ambiguous, failing, unbound, not-run, vacuous, unstated and all`)
		})
	})

	t.When("--fail-on names failing without --input or --verify", func(w *gotest.T) {
		w.It("refuses it with exit 2, naming --input and --verify", func(it *gotest.T) {
			gotest.Equal(it, 2, s.needsRun.Exit)
			gotest.Empty(it, s.needsRun.Stdout)
			gotest.Contains(it, s.needsRun.Stderr, "--input=<events file> or --verify")
		})
	})

	t.When("a package of the module does not compile", func(w *gotest.T) {
		w.It("names it and exits 2", func(it *gotest.T) {
			gotest.Equal(it, 2, s.broken.Exit)
			gotest.Contains(it, s.broken.Stderr, "scenarios: scenmod/broken cannot be read")
			gotest.Contains(it, s.broken.Stdout, "unreadable scenmod/broken\n")
		})
	})
}
