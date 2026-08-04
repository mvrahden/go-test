package main

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

// detectCIEnv reports whether CI mode should be active without an explicit
// --ci flag: GOTEST_CI opts in unless explicitly falsy ("0"/"false"), and an
// unset GOTEST_CI falls back to the standard CI variable with the same rule —
// so a typo'd opt-in never silently disables the CI gate.
func detectCIEnv() bool {
	if v := os.Getenv("GOTEST_CI"); v != "" {
		return !isFalsyEnv(v)
	}
	v := os.Getenv("CI")
	return v != "" && !isFalsyEnv(v)
}

func isFalsyEnv(v string) bool {
	return v == "0" || v == "false"
}

type FocusViolation struct {
	SuiteName  string
	MethodName string
	Pos        string // "file.go:line", empty when unknown
}

func (v FocusViolation) String() string {
	prefix := "  "
	if v.Pos != "" {
		prefix = fmt.Sprintf("  %s  ", v.Pos)
	}
	if v.MethodName != "" {
		return fmt.Sprintf("%s%s.%s", prefix, v.SuiteName, v.MethodName)
	}
	return fmt.Sprintf("%stype %s", prefix, v.SuiteName)
}

// relPosition renders a token position as "file.go:line", relative to the
// working directory when possible.
func relPosition(fset *token.FileSet, pos token.Pos) string {
	if fset == nil || !pos.IsValid() {
		return ""
	}
	p := fset.Position(pos)
	name := p.Filename
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, name); err == nil {
			name = rel
		}
	}
	return fmt.Sprintf("%s:%d", name, p.Line)
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
		fset := s.FileSet()
		if strings.HasPrefix(name, "F_") {
			violations = append(violations, FocusViolation{SuiteName: name, Pos: relPosition(fset, s.TypeSpecPos())})
		}
		for _, tc := range s.TestCases() {
			tcName := tc.Identifier()
			if strings.HasPrefix(tcName, "F_") {
				violations = append(violations, FocusViolation{SuiteName: name, MethodName: tcName, Pos: relPosition(fset, tc.Pos())})
			}
		}
	}
	return violations
}
