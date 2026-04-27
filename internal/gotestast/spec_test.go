package gotestast

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestRegexp(t *testing.T) {
	testCases := []struct {
		desc string
		fn   *regexpW
		in   []string
		out  []string
	}{
		{"test case rejects unexported names", IS_TEST_CASE, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{"test case rejects lowercase prefix", IS_TEST_CASE, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{"test case rejects bare Parallel suffix", IS_TEST_CASE, []string{"TESTParallel", "TestParallel", "_TestParallel", "ABCTestParallelFoo"}, nil},
		{"test case rejects lowercase prefix with Parallel", IS_TEST_CASE, []string{"x_TestParallelFoo", "f_TestParallelFoo"}, nil},
		{"test case matches exported names", IS_TEST_CASE, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}},
		{"suite method rejects unexported names", IS_TEST_SUITE_METHOD, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{"suite method rejects lowercase prefix", IS_TEST_SUITE_METHOD, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{"suite method rejects bare Parallel suffix", IS_TEST_SUITE_METHOD, []string{"TESTParallel", "TestParallel", "_TestParallel", "ABCTestParallelFoo"}, nil},
		{"suite method rejects lowercase prefix with Parallel", IS_TEST_SUITE_METHOD, []string{"x_TestParallelFoo", "f_TestParallelFoo"}, nil},
		{"suite method matches exported names", IS_TEST_SUITE_METHOD, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}},
		{"suite rejects unexported or embedded TestSuite", IS_TEST_SUITE, []string{"TESTSUITE", "TestSuite", "_TestSuite", "ABCTestSuiteABC"}, nil},
		{"suite rejects bare Parallel prefix", IS_TEST_SUITE, []string{"ParallelTESTSUITE", "ABCParallelTestSuiteFoo"}, nil},
		{"suite rejects generated harness names", IS_TEST_SUITE, []string{"ƒƒ_GOTEST_ABC_ParallelTestSuite", "ƒƒ_GOTEST_F_ABC_ParallelTestSuite", "ƒƒ_GOTEST_X_ABC_ParallelTestSuite", "ƒƒ_GOTEST_ABC_ParallelTestSuite"}, nil},
		{"suite matches exported TestSuite types", IS_TEST_SUITE, []string{"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite", "Foo_ParallelTestSuite", "FooParallelTestSuite"}, []string{
			"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite", "Foo_ParallelTestSuite", "FooParallelTestSuite"}},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			var actualMatches []string
			for _, v := range tC.in {
				ok := tC.fn.MatchString(v)
				if ok {
					actualMatches = append(actualMatches, v)
				}
			}
			gotest.Equal(t, tC.out, actualMatches)
		})
	}
}
