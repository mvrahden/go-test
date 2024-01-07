package gotestast

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegexp(t *testing.T) {
	testCases := []struct {
		fn  *regexpW
		in  []string
		out []string
	}{
		// TODO: refine cases (NAME MUST BE EXPORTED, PARALLEL-only Sufffix not allowed)
		{IS_TEST_CASE, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{IS_TEST_CASE, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{IS_TEST_CASE, []string{"TESTParallel", "TestParallel", "_TestParallel", "ABCTestParallelFoo"}, nil},
		{IS_TEST_CASE, []string{"x_TestParallelFoo", "f_TestParallelFoo"}, nil},
		{IS_TEST_CASE, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}},
		// TODO: refine cases (NAME MUST BE EXPORTED)
		{IS_TEST_SUITE_METHOD, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{IS_TEST_SUITE_METHOD, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{IS_TEST_SUITE_METHOD, []string{"TESTParallel", "TestParallel", "_TestParallel", "ABCTestParallelFoo"}, nil},
		{IS_TEST_SUITE_METHOD, []string{"x_TestParallelFoo", "f_TestParallelFoo"}, nil},
		{IS_TEST_SUITE_METHOD, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}},
		// TODO: refine cases (NAME MUST BE EXPORTED, PARALLEL-only Prefix not allowed)
		{IS_TEST_SUITE, []string{"TESTSUITE", "TestSuite", "_TestSuite", "ABCTestSuiteABC"}, nil},
		{IS_TEST_SUITE, []string{"x_TestSuite", "f_TestSuite", "X_TestSuite", "F_TestSuite", "X_ParallelTestSuite", "F_ParallelTestSuite"}, nil},
		{IS_TEST_SUITE, []string{"ParallelTESTSUITE", "ParallelTestSuite", "ABCParallelTestSuiteFoo"}, nil},
		{IS_TEST_SUITE, []string{"x_ParallelTestSuite", "f_TestParallelFoo", "_ParallelTestSuite"}, nil},
		{IS_TEST_SUITE, []string{"ƒƒ_GOTEST_ABC_ParallelTestSuite", "ƒƒ_GOTEST_F_ABC_ParallelTestSuite", "ƒƒ_GOTEST_X_ABC_ParallelTestSuite", "ƒƒ_GOTEST_ABC_ParallelTestSuite"}, nil},
		{IS_TEST_SUITE, []string{"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite", "Foo_ParallelTestSuite", "FooParallelTestSuite"}, []string{
			"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite", "Foo_ParallelTestSuite", "FooParallelTestSuite"}},
	}
	for idx, tC := range testCases {
		t.Run(fmt.Sprintf("Test Case %d", idx+1), func(t *testing.T) {
			var actualMatches []string
			for _, v := range tC.in {
				ok := tC.fn.MatchString(v)
				if ok {
					actualMatches = append(actualMatches, v)
				}
			}
			require.Equal(t, tC.out, actualMatches)
		})
	}
}
