package withfailguard //nolint:stdlib-test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// === error nil guards ===

func TestErrorGuards(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Fail for error nil check`
		gotest.Fail(t, "unexpected error: %v", err)
	}
	if err == nil { // want `use Error instead of if\+Fail for error nil check`
		gotest.Fail(t, "expected an error")
	}
}

// === len guards ===

func TestLenGuards(t *testing.T) {
	items := []string{"a"}
	if len(items) == 0 { // want `use NotEmpty instead of if\+Fail for len == 0 check`
		gotest.Fail(t, "expected at least one item")
	}
	if len(items) > 0 { // want `use Empty instead of if\+Fail for len > 0 check`
		gotest.Fail(t, "expected no items")
	}
	if len(items) != 3 { // want `use Len instead of if\+Fail for len comparison`
		gotest.Fail(t, "expected three items")
	}
}

// === comparison guards ===

func TestComparisonGuards(t *testing.T) {
	a, b := 1, 2
	if a != b { // want `use Equal instead of if\+Fail for != comparison`
		gotest.Fail(t, "a and b differ")
	}
	if a >= b { // want `use Less instead of if\+Fail for >= comparison`
		gotest.Fail(t, "a should be below b")
	}
}

// === nil guards on non-error types ===

func TestNilGuards(t *testing.T) {
	var p *int
	if p == nil { // want `use NotZero instead of if\+Fail for nil check`
		gotest.Fail(t, "expected a pointer")
	}
	var m map[string]int
	if m != nil { // want `use Nil instead of if\+Fail for nil check`
		gotest.Fail(t, "expected no map")
	}
}

// === bool guards, including the False fallback ===

func TestBoolGuards(t *testing.T) {
	ok := false
	if !ok { // want `use True instead of if\+Fail for negation`
		gotest.Fail(t, "should be ok")
	}
	if ok { // want `use False instead of if\+Fail for failure guard`
		gotest.Fail(t, "should not be ok")
	}
}

// === predicate guards ===

var errSentinel = errors.New("sentinel")

func TestPredicateGuards(t *testing.T) {
	s := "hello"
	if strings.Contains(s, "bye") { // want `use NotContains instead of if\+Fail for strings.Contains call`
		gotest.Fail(t, "unexpected substring")
	}
	err := errors.New("boom")
	if !errors.Is(err, errSentinel) { // want `use ErrorIs instead of if\+Fail for errors.Is call`
		gotest.Fail(t, "wrong error")
	}
	if s == "" { // want `use NotEmpty instead of if\+Fail for empty string check`
		gotest.Fail(t, "expected content")
	}
}

// === message args containing calls: report, but no autofix — the if body
// evaluates them only on failure, an assertion would evaluate them always ===

func TestMessageArgGate(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Fail for error nil check`
		gotest.Fail(t, "unexpected error: %v", describe(err))
	}
}

func describe(err error) string { return err.Error() }

// === shapes that must not be flagged ===

type failer struct{}

func (failer) Fail(t *testing.T, msg string, args ...any) {}

func TestNotFlagged(t *testing.T) {
	err := errors.New("boom")
	// body has more than the fail call
	if err != nil {
		t.Log("saw error")
		gotest.Fail(t, "unexpected error")
	}
	// else branch present
	if err != nil {
		gotest.Fail(t, "unexpected error")
	} else {
		t.Log("no error")
	}
	// init statement scopes the value to the if
	if err := errors.New("x"); err != nil { // want `use NoError instead of if\+Fail for error nil check`
		gotest.Fail(t, "unexpected error")
	}
	// same-name method on another type
	var f failer
	if err != nil {
		f.Fail(t, "unexpected error")
	}
}

// === || guards decompose into sequential assertions — halting on the first
// failure reproduces the short-circuit exactly ===

func TestOrGuards(t *testing.T) {
	err := errors.New("boom")
	items := []string{"a"}
	if err != nil || len(items) == 0 { // want `use NoError \+ NotEmpty instead of if\+Fail for or-chained failure guard`
		gotest.Fail(t, "load failed: %v", err)
	}
}

// === else-if Fail chains decompose with per-branch messages ===

func TestElseIfChains(t *testing.T) {
	err := errors.New("boom")
	items := []string{"a"}
	if err != nil { // want `use NoError \+ NotEmpty instead of if\+Fail for else-if failure guard`
		gotest.Fail(t, "unexpected error: %v", err)
	} else if len(items) == 0 {
		gotest.Fail(t, "expected items")
	}
}

// === && does not decompose (the negation is a disjunction) — only the
// False fallback is sound ===

func TestAndGuards(t *testing.T) {
	err := errors.New("boom")
	fatal := true
	if err != nil && fatal { // want `use False instead of if\+Fail for failure guard`
		gotest.Fail(t, "fatal error")
	}
}

// === Fail followed by a bare return — unreachable after the halt ===

func TestFailReturn(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Fail for error nil check`
		gotest.Fail(t, "unexpected: %v", err)
		return
	}
}

// === halting testing.T bodies rewrite the same way ===

func TestFatalGuards(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Fatal for error nil check`
		t.Fatal(err)
	}
	items := []string{}
	if len(items) == 0 { // want `use NotEmpty instead of if\+Fatalf for len == 0 check`
		t.Fatalf("no items: %v", err)
	}
	ok := true
	if !ok { // want `use True instead of if\+FailNow for negation`
		t.FailNow()
	}
}

// === non-halting bodies: Errorf continues where an assertion halts —
// report only, never autofix ===

func TestErrorfGuards(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Errorf for error nil check — assertions halt where Errorf continues`
		t.Errorf("unexpected: %v", err)
	}
}

// === message args that can panic under eager evaluation (index, selector,
// deref) block the autofix — errs[0] is valid exactly when the guard fires ===

func TestMessagePanicGate(t *testing.T) {
	var errs []error
	if len(errs) > 0 { // want `use Empty instead of if\+Fail for len > 0 check`
		gotest.Fail(t, "first error: %v", errs[0])
	}
}

// === Errorf+return still deserves the pointer, but without the false claim
// that Errorf continues, and never a fix ===

func TestErrorfReturnGuard(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Errorf for error nil check`
		t.Errorf("unexpected error")
		return
	}
}

// === print-style Fatal args beyond one cannot ride printf-style msgAndArgs ===

func TestFatalPrintArgs(t *testing.T) {
	a, b := 1, 2
	if a != b { // want `use Equal instead of if\+Fatal for != comparison`
		t.Fatal("expected", a, "got", b)
	}
}

// === a lone literal operand belongs in Equal's expected slot ===

func TestLiteralFirst(t *testing.T) {
	got := 7
	if got != 3 { // want `use Equal instead of if\+Fail for != comparison`
		gotest.Fail(t, "wrong count")
	}
	if got != -1 { // want `use Equal instead of if\+Fail for != comparison`
		gotest.Fail(t, "negative")
	}
}

// === comments inside the guard body would be destroyed by the rewrite ===

func TestCommentedGuard(t *testing.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Fail for error nil check`
		// teardown failures here indicate a leaked fixture
		gotest.Fail(t, "unexpected error")
	}
}

// === guards on escaped t.T() receivers belong to the t-escape rule ===

type EscapeGuardTestSuite struct{}

func (s *EscapeGuardTestSuite) TestEscape(it *gotest.T) {
	err := errors.New("boom")
	if err != nil {
		it.T().Fatalf("unexpected: %v", err) // want `use assertions instead — T.Fatalf bypasses the assertion tracer`
	}
}

func (s *EscapeGuardTestSuite) TestEscapeError(it *gotest.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Error for error nil check — assertions halt where Error continues`
		it.T().Error("boom")
	}
}

// === a weak+return branch makes the chain report-only, never invisible ===

func TestChainWithWeakReturn(t *testing.T) {
	a, b := 1, 2
	err := errors.New("boom")
	if a != b { // want `use Equal \+ NoError instead of if\+Fatal for else-if failure guard`
		t.Fatal("mismatch")
	} else if err != nil {
		t.Errorf("boom: %v", err)
		return
	}
}

// A guard whose body t-escape owns stands fail-guard down even inside a
// package-level var function literal.
var packageGuard = func(t *gotest.T) {
	err := errors.New("boom")
	if err != nil {
		t.T().FailNow() // want `FailNow is available on gotest.T — unnecessary T escape`
	}
}
