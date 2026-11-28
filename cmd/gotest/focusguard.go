package main

import (
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
)

// detectCIEnv reports whether CI mode should be active without an explicit
// --ci flag: GOTEST_CI as protocol.SwitchOn reads it, and an unset GOTEST_CI
// falls back to the standard CI variable with the same rule — so a typo'd
// opt-in never silently disables the CI gate.
func detectCIEnv() bool {
	if v := os.Getenv(protocol.EnvCI); v != "" {
		return protocol.SwitchOn(v)
	}
	return protocol.SwitchOn(os.Getenv("CI"))
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

// focusGate is the focus guard of a run: in CI mode enforceFocusGuard;
// outside it a note on stderr that F_ narrows the run, which never fails it.
// A collection error outside CI mode is left to generation, which reports it
// with its location.
func focusGate(loaded []*gotestgen.LoadResult, ci bool) (int, error) {
	if ci {
		return enforceFocusGuard(loaded)
	}
	suites, err := gotestgen.CollectFromLoaded(loaded)
	if err != nil {
		return 0, nil
	}
	if violations := CheckFocusViolations(suites); len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "note: F_ narrows this run to the focused suites and methods (%d, first %s) — the rest is skipped; remove F_ to run everything, --ci refuses it\n", len(violations), strings.Join(strings.Fields(violations[0].String()), " "))
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
		// A focused method of any kind focuses the run.
		for _, methods := range [][]*gotestast.TestSuiteMethod{s.TestCases(), s.Benchmarks(), s.Fuzzers()} {
			for _, m := range methods {
				if m.IsFocused() {
					violations = append(violations, FocusViolation{SuiteName: name, MethodName: m.Identifier(), Pos: relPosition(fset, m.Pos())})
				}
			}
		}
	}
	return violations
}

// noteGenerateFocus says on w that generate ignored the F_ prefixes among
// loaded. A collection error is left to generation.
func noteGenerateFocus(w io.Writer, loaded []*gotestgen.LoadResult) {
	suites, err := gotestgen.CollectFromLoaded(loaded)
	if err != nil {
		return
	}
	violations := CheckFocusViolations(suites)
	if len(violations) == 0 {
		return
	}
	fmt.Fprintf(w, "note: generate ignores F_: each harness holds every suite and method its package declares (%d focused, first %s) — go test runs them all, and a gotest run narrows to the focus\n", len(violations), strings.Join(strings.Fields(violations[0].String()), " "))
}
