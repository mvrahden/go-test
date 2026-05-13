package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

type FocusViolation struct {
	SuiteName  string
	MethodName string
}

func (v FocusViolation) String() string {
	if v.MethodName != "" {
		return fmt.Sprintf("  %s.%s", v.SuiteName, v.MethodName)
	}
	return fmt.Sprintf("  type %s", v.SuiteName)
}

func enforceFocusGuard(loaded []*gotestgen.LoadResult) (int, error) {
	suites, err := gotestgen.CollectFromLoaded(loaded)
	if err != nil {
		return 0, err
	}
	violations := CheckFocusViolations(suites)
	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, v.String())
		}
		return 1, nil
	}
	return 0, nil
}

func CheckFocusViolations(suites gotestast.TestSuiteSpecSet) []FocusViolation {
	var violations []FocusViolation
	for _, s := range suites {
		name := s.Identifier()
		if strings.HasPrefix(name, "F_") {
			violations = append(violations, FocusViolation{SuiteName: name})
		}
		for _, tc := range s.TestCases() {
			tcName := tc.Identifier()
			if strings.HasPrefix(tcName, "F_") {
				violations = append(violations, FocusViolation{SuiteName: name, MethodName: tcName})
			}
		}
	}
	return violations
}
