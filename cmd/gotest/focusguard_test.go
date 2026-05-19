package main //nolint:stdlib-test

import (
	"testing"
)

func TestFocusViolation_String(t *testing.T) {
	tests := []struct {
		name     string
		v        FocusViolation
		expected string
	}{
		{
			name:     "suite violation only",
			v:        FocusViolation{SuiteName: "F_MyTestSuite"},
			expected: "  type F_MyTestSuite",
		},
		{
			name:     "method violation",
			v:        FocusViolation{SuiteName: "MyTestSuite", MethodName: "F_TestSomething"},
			expected: "  MyTestSuite.F_TestSomething",
		},
		{
			name:     "both focused suite and method",
			v:        FocusViolation{SuiteName: "F_MyTestSuite", MethodName: "F_TestFoo"},
			expected: "  F_MyTestSuite.F_TestFoo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.String()
			if got != tt.expected {
				t.Errorf("FocusViolation.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}
