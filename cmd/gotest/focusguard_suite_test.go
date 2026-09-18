package main_test

import (
	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FocusGuardTestSuite covers how a leftover F_ focus is reported.
type FocusGuardTestSuite struct{}

func (s *FocusGuardTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FocusGuardTestSuite) TestFocusViolation_String(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		v        FocusViolation
		expected string
	}{
		{
			Desc:     "suite violation only",
			v:        FocusViolation{SuiteName: "F_MyTestSuite"},
			expected: "  type F_MyTestSuite",
		},
		{
			Desc:     "method violation",
			v:        FocusViolation{SuiteName: "MyTestSuite", MethodName: "F_TestSomething"},
			expected: "  MyTestSuite.F_TestSomething",
		},
		{
			Desc:     "both focused suite and method",
			v:        FocusViolation{SuiteName: "F_MyTestSuite", MethodName: "F_TestFoo"},
			expected: "  F_MyTestSuite.F_TestFoo",
		},
		{
			Desc:     "suite violation with position",
			v:        FocusViolation{SuiteName: "F_MyTestSuite", Pos: "pkg/user/user_test.go:12"},
			expected: "  pkg/user/user_test.go:12  type F_MyTestSuite",
		},
		{
			Desc:     "method violation with position",
			v:        FocusViolation{SuiteName: "MyTestSuite", MethodName: "F_TestSomething", Pos: "pkg/user/user_test.go:28"},
			expected: "  pkg/user/user_test.go:28  MyTestSuite.F_TestSomething",
		},
	}) {
		gotest.Equal(sub, tc.expected, tc.v.String())
	}
}
