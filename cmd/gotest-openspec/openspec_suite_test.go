package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// specJSON is a two-suite spec tree as gotest spec --format=json emits it,
// trimmed to the fields the reconciliation reads.
const specJSON = `{"packages":[{"nodes":[
  {"display":"Cart","vocab":"","status":"pass","children":[
    {"display":"ApplyDiscount","vocab":"","status":"pass","children":[
      {"display":"when the discount exceeds 100 percent","vocab":"when","status":"pass","children":[
        {"display":"clamps to zero","vocab":"it","status":"pass","children":[]}]}]},
    {"display":"Checkout","vocab":"","status":"fail","children":[
      {"display":"when the cart is empty","vocab":"when","status":"fail","children":[
        {"display":"returns an error","vocab":"it","status":"fail","children":[]}]}]}]},
  {"display":"Pricing","vocab":"","status":"none","children":[
    {"display":"LookupPrice","vocab":"","status":"none","children":[
      {"display":"returns the catalog price","vocab":"it","status":"none","children":[]}]}]}
]}]}`

type ReconcileTestSuite struct {
	specs string
	out   bytes.Buffer
	errs  bytes.Buffer
}

func (s *ReconcileTestSuite) BeforeEach(t *gotest.T) {
	s.specs = t.TempDir()
	s.out.Reset()
	s.errs.Reset()
}

func (s *ReconcileTestSuite) write(t *gotest.T, name, body string) {
	path := filepath.Join(s.specs, name)
	gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	gotest.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func (s *ReconcileTestSuite) reconcile(spec string) int {
	return run(strings.NewReader(spec), &s.out, &s.errs, []string{s.specs})
}

func (s *ReconcileTestSuite) TestScenarioHeadings(t *gotest.T) {
	s.write(t, "cart/spec.md", "### Requirement: A\n#### Scenario: clamps to zero\n- **THEN** x\n")
	s.write(t, "nested/deeper/spec.md", "### Scenario: returns the catalog price\n")
	s.write(t, "notes.txt", "#### Scenario: ignored, not markdown\n")

	names, err := scenarios(s.specs)
	gotest.NoError(t, err)

	t.It("collects level-three and level-four headings from every markdown file", func(t *gotest.T) {
		gotest.ElementsMatch(t, []string{"clamps to zero", "returns the catalog price"}, names)
	})
}

func (s *ReconcileTestSuite) TestMatching(t *gotest.T) {
	t.When("the scenario title equals an It label", func(t *gotest.T) {
		s.write(t, "spec.md", "#### Scenario: Clamps to zero.\n")
		code := s.reconcile(specJSON)
		t.It("verifies it despite case and punctuation", func(t *gotest.T) {
			gotest.Equal(t, 0, code)
			gotest.Contains(t, s.out.String(), "✓ verified     Clamps to zero.")
		})
	})

	t.When("the scenario title spells the condition before the outcome", func(t *gotest.T) {
		s.write(t, "spec.md", "#### Scenario: the discount exceeds 100 percent clamps to zero\n")
		code := s.reconcile(specJSON)
		t.It("matches the When condition followed by the It label", func(t *gotest.T) {
			gotest.Equal(t, 0, code)
			gotest.Contains(t, s.out.String(), "1 scenarios: 1 verified, 0 failing, 0 declared, 0 without a behavior")
		})
	})

	t.When("the behavior failed", func(t *gotest.T) {
		s.write(t, "spec.md", "#### Scenario: returns an error\n")
		code := s.reconcile(specJSON)
		t.It("reports it failing and exits 1", func(t *gotest.T) {
			gotest.Equal(t, 1, code)
			gotest.Contains(t, s.out.String(), "✗ failing      returns an error")
		})
	})

	t.When("the tree is static and carries no verdicts", func(t *gotest.T) {
		s.write(t, "spec.md", "#### Scenario: returns the catalog price\n")
		code := s.reconcile(specJSON)
		t.It("reports the scenario as declared and exits 0", func(t *gotest.T) {
			gotest.Equal(t, 0, code)
			gotest.Contains(t, s.out.String(), "· declared     returns the catalog price")
		})
	})

	t.When("no behavior carries the scenario's title", func(t *gotest.T) {
		s.write(t, "spec.md", "#### Scenario: applies a coupon code\n")
		code := s.reconcile(specJSON)
		t.It("reports it without a behavior and exits 1", func(t *gotest.T) {
			gotest.Equal(t, 1, code)
			gotest.Contains(t, s.out.String(), "? no behavior  applies a coupon code")
			gotest.Contains(t, s.out.String(), "1 without a behavior")
		})
	})
}

func (s *ReconcileTestSuite) TestInputs(t *gotest.T) {
	t.When("the specs directory is missing", func(t *gotest.T) {
		code := run(strings.NewReader(specJSON), &s.out, &s.errs, []string{filepath.Join(s.specs, "absent")})
		t.It("exits 2 and says so", func(t *gotest.T) {
			gotest.Equal(t, 2, code)
			gotest.Contains(t, s.errs.String(), "read scenarios")
		})
	})

	t.When("stdin is not a spec tree", func(t *gotest.T) {
		code := s.reconcile("not json")
		t.It("exits 2 and says so", func(t *gotest.T) {
			gotest.Equal(t, 2, code)
			gotest.Contains(t, s.errs.String(), "read spec json")
		})
	})

	t.When("no directory is given", func(t *gotest.T) {
		code := run(strings.NewReader(specJSON), &s.out, &s.errs, nil)
		t.It("prints usage and exits 2", func(t *gotest.T) {
			gotest.Equal(t, 2, code)
			gotest.Contains(t, s.errs.String(), "usage:")
		})
	})
}
