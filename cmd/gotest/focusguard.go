package main

import (
	"fmt"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
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
