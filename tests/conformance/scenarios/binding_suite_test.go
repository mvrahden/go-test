package scenarios_test

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScenariosBindingTestSuite: a scenario binds to the When and It labels
// that state its text, read from source, and what does not bind once is
// reported.
type ScenariosBindingTestSuite struct {
	dir                 string
	bound, withUnstated testkit.Run
	doc, unstatedDoc    document
}

const cartSpec = "specs/cart/spec.md"

func (s *ScenariosBindingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *ScenariosBindingTestSuite) BeforeAll(t *gotest.T) {
	s.dir = tempDir(t, "gotest-scenarios-binding-")
	mod := stage(t, s.dir, "mod")
	s.bound = scenarios(t, mod, "--format=json")
	s.doc = readDoc(t, &s.bound)
	s.withUnstated = scenarios(t, mod, "--format=json", "--unstated")
	s.unstatedDoc = readDoc(t, &s.withUnstated)
}

func (s *ScenariosBindingTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *ScenariosBindingTestSuite) TestBindingByText(t *gotest.T) {
	t.When("a scenario's texts equal a When and its Its, but for the first letter's case, backticks and a trailing period", func(w *gotest.T) {
		w.It("binds the scenario, naming the file and line of each clause's It, and exits 0 without running a test", func(it *gotest.T) {
			gotest.Equal(it, 0, s.bound.Exit, "stderr: %s", s.bound.Stderr)
			sc := byTitle(it, &s.doc, cartSpec, "An empty cart")
			gotest.Equal(it, "bound", sc.Status)
			gotest.Equal(it, "The cart is `empty`.", sc.Conditions[0].Text)
			gotest.Len(it, sc.Clauses, 2)
			gotest.Equal(it, []string{"shop/cart_suite_test.go:17"}, at(sc.Clauses[0].Its))
			gotest.Equal(it, []string{"shop/cart_suite_test.go:20"}, at(sc.Clauses[1].Its))
			gotest.Equal(it, "none", s.doc.Evidence)
			gotest.Zero(it, s.doc.Run)
		})
	})

	t.When("a scenario has a GIVEN", func(w *gotest.T) {
		w.It("binds it only where its When sits directly in a When labelled with the GIVEN text", func(it *gotest.T) {
			sc := byTitle(it, &s.doc, cartSpec, "A member's discount")
			gotest.Equal(it, "bound", sc.Status)
			gotest.Equal(it, []string{"shop/cart_suite_test.go:28"}, at(sc.Whens))
			gotest.Equal(it, []string{"shop/cart_suite_test.go:29"}, at(sc.Clauses[0].Its))
		})
	})

	t.When("a scenario's WHEN text is no When's label, or a THEN text is no It's under it", func(w *gotest.T) {
		w.It("lists the scenario as unbound, naming the clause that has no When or no It", func(it *gotest.T) {
			refund := byTitle(it, &s.doc, cartSpec, "A refund")
			gotest.Equal(it, "unbound", refund.Status)
			gotest.Empty(it, refund.Whens)
			coupon := byTitle(it, &s.doc, cartSpec, "A coupon")
			gotest.Equal(it, "unbound", coupon.Status)
			gotest.Equal(it, []string{"shop/cart_suite_test.go:39"}, at(coupon.Whens))
			gotest.Equal(it, "bound", coupon.Clauses[0].Status)
			gotest.Equal(it, "unbound", coupon.Clauses[1].Status)
			gotest.Equal(it, "the coupon is used up", coupon.Clauses[1].Text)
			gotest.Empty(it, coupon.Clauses[1].Its)
		})
	})

	t.When("two scenarios differ only in letters outside ASCII", func(w *gotest.T) {
		w.It("binds each to the When that spells it and not to the other", func(it *gotest.T) {
			empty := byTitle(it, &s.doc, cartSpec, "空のカート")
			full := byTitle(it, &s.doc, cartSpec, "満杯のカート")
			gotest.Equal(it, "bound", empty.Status)
			gotest.Equal(it, "bound", full.Status)
			gotest.Equal(it, []string{"shop/cart_suite_test.go:45"}, at(empty.Clauses[0].Its))
			gotest.Equal(it, []string{"shop/cart_suite_test.go:50"}, at(full.Clauses[0].Its))
		})
	})

	t.When("the only It with a clause's text is under an Each row and not a When, or in a suite X_ excludes", func(w *gotest.T) {
		w.It("lists the scenario as unbound", func(it *gotest.T) {
			for _, title := range []string{"Rows of a table", "A parked check"} {
				sc := byTitle(it, &s.doc, cartSpec, title)
				gotest.Equal(it, "unbound", sc.Status, title)
				gotest.Empty(it, sc.Whens, title)
			}
		})
	})
}

func (s *ScenariosBindingTestSuite) TestAmbiguity(t *gotest.T) {
	t.When("a clause's text equals Its declared at two places under Whens with the scenario's text", func(w *gotest.T) {
		w.It("lists the scenario as ambiguous, naming the file and line of both Its", func(it *gotest.T) {
			sc := byTitle(it, &s.doc, cartSpec, "Stock counted twice")
			gotest.Equal(it, "ambiguous", sc.Status)
			gotest.Equal(it, []string{"shop/stock_suite_test.go:17", "shop/stock_suite_test.go:25"}, at(sc.Clauses[0].Its))
		})
	})

	t.When("two capabilities hold a scenario with the same WHEN and THEN texts", func(w *gotest.T) {
		w.It("lists both scenarios as ambiguous", func(it *gotest.T) {
			gotest.Equal(it, "ambiguous", byTitle(it, &s.doc, cartSpec, "A price").Status)
			gotest.Equal(it, "ambiguous", byTitle(it, &s.doc, "openspec/specs/stock/spec.md", "A price on the shelf").Status)
		})
	})
}

func (s *ScenariosBindingTestSuite) TestBehaviorsNoClauseStates(t *gotest.T) {
	t.When("a bound When holds an It no clause states and --unstated is given", func(w *gotest.T) {
		w.It("lists that It as unstated with its file and line", func(it *gotest.T) {
			gotest.Equal(it, []string{"shop/cart_suite_test.go:23"}, at(s.unstatedDoc.Unstated))
			gotest.Equal(it, []string{"TestCartTestSuite", "TestTotals", "the cart is empty", "logs nothing"}, s.unstatedDoc.Unstated[0].Path)
		})
	})

	t.When("a bound When holds an It no clause states and --unstated is not given", func(w *gotest.T) {
		w.It("lists no unstated behavior", func(it *gotest.T) {
			gotest.Empty(it, s.doc.Unstated)
			gotest.Zero(it, s.doc.Summary.Unstated)
		})
	})
}
