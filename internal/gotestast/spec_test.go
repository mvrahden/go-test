package gotestast

import (
	"fmt"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/require"
)

func TestRegexp(t *testing.T) {
	testCases := []struct {
		fn  *regexp2.Regexp
		in  []string
		out []string
	}{
		{IS_TEST_CASE, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{IS_TEST_CASE, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{IS_TEST_CASE, []string{"TESTParallel", "TestParallel", "_TestParallel", "ABCTestParallelFoo"}, nil},
		{IS_TEST_CASE, []string{"x_TestParallelFoo", "f_TestParallelFoo"}, nil},
		{IS_TEST_CASE, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}},
		{IS_TEST_HARNESS_METHOD, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{IS_TEST_HARNESS_METHOD, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{IS_TEST_HARNESS_METHOD, []string{"TESTParallel", "TestParallel", "_TestParallel", "ABCTestParallelFoo"}, nil},
		{IS_TEST_HARNESS_METHOD, []string{"x_TestParallelFoo", "f_TestParallelFoo"}, nil},
		{IS_TEST_HARNESS_METHOD, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo", "TestParallelFoo"}},
	}
	for idx, tC := range testCases {
		t.Run(fmt.Sprintf("Test Case %d", idx+1), func(t *testing.T) {
			var actualMatches []string
			for _, v := range tC.in {
				ok, err := tC.fn.MatchString(v)
				require.NoError(t, err)
				if ok {
					actualMatches = append(actualMatches, v)
				}
			}
			require.Equal(t, tC.out, actualMatches)
		})
	}
}
