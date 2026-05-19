package gotestast //nolint:stdlib-test

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
		{"test case matches exported names", IS_TEST_CASE, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}},
		{"suite method rejects unexported names", IS_TEST_SUITE_METHOD, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{"suite method rejects lowercase prefix", IS_TEST_SUITE_METHOD, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{"suite method matches exported names", IS_TEST_SUITE_METHOD, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}},
		{"suite rejects unexported or embedded TestSuite", IS_TEST_SUITE, []string{"TESTSUITE", "TestSuite", "_TestSuite", "ABCTestSuiteABC"}, nil},
		{"suite rejects generated harness names", IS_TEST_SUITE, []string{"ƒƒ_GOTEST_ABCTestSuite", "ƒƒ_GOTEST_F_ABCTestSuite"}, nil},
		{"suite matches exported TestSuite types", IS_TEST_SUITE, []string{"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite"}, []string{
			"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite"}},
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
