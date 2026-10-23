package gotestast_test

import (
	"os"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// HookCallsTestSuite pins the rejection of hooks calling t.T() where the
// fuzz or benchmark wrapper hands them a nil *testing.T.
type HookCallsTestSuite struct{}

type hookCallRow struct {
	Desc  string
	suite string
	want  string // error substring; empty when the suite is accepted
}

func (s *HookCallsTestSuite) TestHooksCallingT(t *gotest.T) {
	pkg := loadTestdataPkg(t.T(), "./testdata/hookcalls")
	byName := map[string]*gotestast.TestSuiteSpec{}
	for _, ts := range collectHarvestSuites(t.T(), pkg) {
		byName[ts.Identifier()] = ts
	}
	src, err := os.ReadFile("testdata/hookcalls/hooks_test.go")
	gotest.NoError(t, err)
	lines := strings.Split(string(src), "\n")
	markerLine := func(suite string) int {
		for i, l := range lines {
			if strings.HasSuffix(l, "// at:"+suite) {
				return i + 1
			}
		}
		return 0
	}

	t.It("refuses each hook the wrappers hand a nil *testing.T, naming them in source order at the first", func(it *gotest.T) {
		for sub, row := range gotest.Each(it, []hookCallRow{
			{Desc: "fuzz skips the per-input hooks", suite: "SeededTestSuite", want: "has fuzz methods but BeforeAll and AfterAll call t.T()"},
			{Desc: "benchmark covers all four hooks", suite: "TimedTestSuite", want: "has benchmark methods but AfterEach, BeforeAll, BeforeEach and AfterAll call t.T()"},
			{Desc: "both wrappers nil BeforeAll", suite: "MixedAllTestSuite", want: "has fuzz and benchmark methods but BeforeAll calls t.T()"},
			{Desc: "only the benchmark nils BeforeEach", suite: "MixedEachTestSuite", want: "has benchmark methods but BeforeEach and AfterAll call t.T()"},
			{Desc: "parenthesised receiver", suite: "ParenTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "method value", suite: "MethodValueTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "method value as argument", suite: "MethodArgTestSuite", want: "has fuzz methods but AfterAll calls t.T()"},
			{Desc: "alias by define", suite: "AliasDefineTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "alias by var and assignment", suite: "AliasVarTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "closure in the hook", suite: "ClosureTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "result checked outside an if-init", suite: "UnprovenBindTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "bound result used where nil", suite: "WrongBranchTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
			{Desc: "call in the nil branch", suite: "NilBranchTestSuite", want: "has fuzz methods but BeforeAll calls t.T()"},
		}) {
			ts := byName[row.suite]
			gotest.NotZero(sub, ts, "suite %s not collected", row.suite)
			for range 20 {
				err := gotestast.ValidateContextConsistency(ts)
				gotest.ErrorContains(sub, err, "suite "+row.suite+" "+row.want+", which is nil under them")
				gotest.Equal(sub, markerLine(row.suite), pkg.Fset.Position(gotestast.PosOf(err)).Line)
			}
		}
	})

	t.It("accepts every t.T() that is not the hook's own, under a real *testing.T, or guarded", func(it *gotest.T) {
		for sub, row := range gotest.Each(it, []hookCallRow{
			{Desc: "fuzz per-input hooks", suite: "FuzzEachTestSuite"},
			{Desc: "shadowed t in a func literal", suite: "ShadowTestSuite"},
			{Desc: "another *gotest.T", suite: "OtherTTestSuite"},
			{Desc: "T() of an unrelated type", suite: "UnrelatedTTestSuite"},
			{Desc: "nil comparisons and checked branches", suite: "GuardedTestSuite"},
			{Desc: "a helper's call is not followed", suite: "HelperTestSuite"},
		}) {
			ts := byName[row.suite]
			gotest.NotZero(sub, ts, "suite %s not collected", row.suite)
			gotest.NoError(sub, gotestast.ValidateContextConsistency(ts))
		}
	})

	t.It("refuses a returning BeforeEach before a hook calling t.T()", func(it *gotest.T) {
		for sub, row := range gotest.Each(it, []hookCallRow{
			{Desc: "fuzz", suite: "ReturningFuzzTestSuite", want: "has fuzz methods but a returning BeforeEach"},
			{Desc: "benchmark", suite: "ReturningBenchTestSuite", want: "has benchmark methods but a returning BeforeEach"},
		}) {
			ts := byName[row.suite]
			gotest.NotZero(sub, ts, "suite %s not collected", row.suite)
			gotest.ErrorContains(sub, gotestast.ValidateContextConsistency(ts), row.want)
		}
	})
}
