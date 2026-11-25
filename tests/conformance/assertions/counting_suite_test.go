package assertions_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AssertionCountTestSuite: each leaf reports how many assertions it made,
// and a test may declare that it asserts by other means.
type AssertionCountTestSuite struct{}

func (s *AssertionCountTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

const (
	countPkg   = "../testdata/assertcount/"
	countRed   = "../testdata/assertcountred/"
	countSuite = "TestCountTestSuite/"
)

func (s *AssertionCountTestSuite) TestCountingAssertions(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), countPkg)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	count := func(test string) (string, bool) { return run.Attr(test, protocol.AttrAssertions) }

	t.When("a leaf asserts on its T, a soft T, its *testing.T, Ts made from them, in a helper, on a gotest.Go goroutine, in a poll and with TimeIsNow", func(w *gotest.T) {
		w.It("reports one assertion per call, none for what the poll's R asserts", func(it *gotest.T) {
			n, ok := count(countSuite + "TestEverywhere")
			gotest.True(it, ok, "no count reported")
			gotest.Equal(it, "8", n)
			n, _ = count(countSuite + "TestTwo")
			gotest.Equal(it, "2", n)
		})
	})
	t.When("a method asserts only in BeforeEach and AfterEach", func(w *gotest.T) {
		w.It("reports the hooks' two assertions as the method's", func(it *gotest.T) {
			n, _ := count("TestHookTestSuite/TestBodyless")
			gotest.Equal(it, "2", n)
		})
	})
	t.When("a method's When holds Its and its Each table rows", func(w *gotest.T) {
		w.It("each It and each row reports its own count, and the method and the When report none", func(it *gotest.T) {
			for test, want := range map[string]string{
				"TestBehaviors/a_When_holds_Its/asserts":         "1",
				"TestBehaviors/a_When_holds_Its/asserts_nothing": "0",
				"TestRows/asserting_row":                         "1",
				"TestRows/silent_row":                            "0",
			} {
				n, ok := count(countSuite + test)
				gotest.True(it, ok, "%s reported no count", test)
				gotest.Equal(it, want, n, test)
			}
			for _, test := range []string{"TestBehaviors", "TestBehaviors/a_When_holds_Its", "TestRows"} {
				_, ok := count(countSuite + test)
				gotest.False(it, ok, "%s reported a count", test)
			}
		})
		w.It("an It that asserts only on the method's T reports 0", func(it *gotest.T) {
			n, _ := count(countSuite + "TestBehaviors/a_When_holds_Its/asserts_on_the_method's_T")
			gotest.Equal(it, "0", n)
		})
		w.It("a method whose Each table is empty reports 0", func(it *gotest.T) {
			n, ok := count(countSuite + "TestNoRows")
			gotest.True(it, ok, "no count reported")
			gotest.Equal(it, "0", n)
		})
	})
	t.When("a test skips", func(w *gotest.T) {
		w.It("reports no count", func(it *gotest.T) {
			gotest.Equal(it, "skip", run.Verdict(countSuite+"TestSkipped"))
			_, ok := count(countSuite + "TestSkipped")
			gotest.False(it, ok)
		})
	})
}

func (s *AssertionCountTestSuite) TestDeclaringHowATestAsserts(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), countPkg)
	gotest.NoError(t, err)
	red, err := testkit.RunPackage(t.Context(), t.TempDir(), countRed)
	gotest.NoError(t, err)
	t.When("a leaf declares a reason and makes no assertion", func(w *gotest.T) {
		w.It("reports the reason and a count of 0", func(it *gotest.T) {
			by, _ := run.Attr(countSuite+"TestDeclared", protocol.AttrAssertsBy)
			gotest.Equal(it, "the call must not panic", by)
			n, _ := run.Attr(countSuite+"TestDeclared", protocol.AttrAssertions)
			gotest.Equal(it, "0", n)
		})
	})
	t.When("a method declares a reason and then opens an It", func(w *gotest.T) {
		w.It("the It reports the method's reason", func(it *gotest.T) {
			by, _ := run.Attr(countSuite+"TestDeclaredAbove/is_covered", protocol.AttrAssertsBy)
			gotest.Equal(it, "a mock controller given T()", by)
		})
	})
	t.When("a test declares an empty reason", func(w *gotest.T) {
		w.It("fails at the line of the call", func(it *gotest.T) {
			gotest.Equal(it, "fail", red.Verdict("TestEmptyReasonTestSuite/TestEmpty"))
			gotest.Regexp(it, `assertcountred_suite_test\.go:\d+: AssertsBy: the reason is empty`, red.Stdout)
		})
	})
}
