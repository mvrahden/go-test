package main_test

import (
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/gotestgen"
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

func (s *FocusGuardTestSuite) TestFocusGate(t *gotest.T) {
	dir, err := filepath.Abs(filepath.Join("..", "..", "tests", "conformance", "testdata", "focused"))
	gotest.NoError(t, err)
	loaded, _, err := gotestgen.LoadPackages([]string{dir}, nil)
	gotest.NoError(t, err)
	t.When("a load holds a focused method", func(w *gotest.T) {
		w.It("fails the run in CI mode", func(it *gotest.T) {
			code, err := ExportFocusGate(loaded, &ExecConfig{CI: true})
			gotest.NoError(it, err)
			gotest.Equal(it, 1, code)
		})
		w.It("lets it run outside CI mode", func(it *gotest.T) {
			cfg := &ExecConfig{}
			code, err := ExportFocusGate(loaded, cfg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, code)
			gotest.True(it, cfg.Focused)
		})
	})
}
