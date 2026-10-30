package gotestast

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
)

// Suites and fixtures are one type declaration each, composed through named
// fields. An embedded type would promote methods gotest reads from a type it
// never discovered, and a suite inside a suite has no lifecycle of its own, so
// both are rejected where they are written, with the alternative named. A
// *TestSuite type must declare a case; a helper is named otherwise.

// interpretedMethod reports whether name is one gotest reads off a suite or
// fixture: a case or a lifecycle hook.
func interpretedMethod(name string) bool {
	switch name {
	case "BeforeAll", "AfterAll", "BeforeEach", "AfterEach",
		"SuiteConfig", "SuiteGuard", "Hydrate", "Dehydrate",
		"FixtureConfig", "SharedFixtureConfig":
		return true
	}
	return IS_TEST_CASE.MatchString(name) || IS_BENCHMARK.MatchString(name) || IS_FUZZ.MatchString(name)
}

// ValidateSuiteShape rejects a suite that embeds a suite, a fixture or a type
// whose method set carries a name gotest reads, or that holds a suite in a
// field. The position is the field's.
func ValidateSuiteShape(s *TestSuiteSpec) (token.Pos, error) {
	return validateFields("suite", s.Identifier(), s.typ, s.pkg.Types)
}

// ValidateDeclaresCase rejects a suite that declares no Test, Benchmark or
// Fuzz method; excluded and unfocused ones count. The position is the suite's
// name.
func ValidateDeclaresCase(s *TestSuiteSpec) (token.Pos, error) {
	if s.th.DeclaresTests || len(s.th.Benchmarks) > 0 || len(s.th.Fuzzers) > 0 {
		return token.NoPos, nil
	}
	return s.Pos(), fmt.Errorf("suite %s declares no Test, Benchmark or Fuzz method; declare one, or give the helper type a name that does not end in TestSuite", s.Identifier())
}

// ValidateFixtureShape is ValidateSuiteShape for a fixture.
func ValidateFixtureShape(f *FixtureSpec) (token.Pos, error) {
	return validateFields("fixture", f.Identifier(), f.typ, f.pkg.Types)
}

func validateFields(kind, owner string, st *types.Struct, in *types.Package) (token.Pos, error) {
	if st == nil {
		return token.NoPos, nil
	}
	for i := range st.NumFields() {
		if err := FieldShapeError(kind, owner, st.Field(i), in); err != nil {
			return st.Field(i).Pos(), err
		}
	}
	return token.NoPos, nil
}

// FieldShapeError is the error a field of the suite or fixture owner is
// rejected with, or nil when the field is accepted. kind is "suite" or
// "fixture".
func FieldShapeError(kind, owner string, field *types.Var, in *types.Package) error {
	named := namedBehind(field.Type())
	if !field.Anonymous() {
		if isSuiteType(named) {
			return fmt.Errorf("%s %s has a field of suite type %s; suites are never nested — share behavior through a generic contract suite", kind, owner, named.Obj().Name())
		}
		return nil
	}
	if named != nil {
		switch name := named.Obj().Name(); {
		case isSuiteType(named):
			return fmt.Errorf("%s %s embeds suite %s; suites are never embedded — share behavior through a generic contract suite", kind, owner, name)
		case strings.HasSuffix(name, suffixFixture):
			return fmt.Errorf("%s %s embeds fixture %s; fixtures compose through a named pointer field", kind, owner, name)
		}
	}
	if m := promotedInterpreted(field.Type()); m != "" {
		return fmt.Errorf("%s %s embeds %s, whose method %s gotest would read; keep helpers in a named field or in functions", kind, owner, types.TypeString(field.Type(), types.RelativeTo(in)), m)
	}
	return nil
}

// isSuiteType reports whether named is a suite: named *TestSuite and
// declaring a case of its own. A case-less one is refused at its own
// declaration, which names the way out, so its field is not refused here.
func isSuiteType(named *types.Named) bool {
	if named == nil || !IS_TEST_SUITE.MatchString(named.Obj().Name()) {
		return false
	}
	origin := named.Origin()
	for i := range origin.NumMethods() {
		name := origin.Method(i).Name()
		if IS_TEST_CASE.MatchString(name) || IS_BENCHMARK.MatchString(name) || IS_FUZZ.MatchString(name) {
			return true
		}
	}
	return false
}

// namedBehind returns the named type t names, through an alias and one
// pointer, or nil.
func namedBehind(t types.Type) *types.Named {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, _ := t.(*types.Named)
	return named
}

// promotedInterpreted names the first method in t's pointer method set that
// gotest would read, or "" when there is none.
func promotedInterpreted(t types.Type) string {
	t = types.Unalias(t)
	if _, ok := t.(*types.Pointer); !ok {
		t = types.NewPointer(t)
	}
	mset := types.NewMethodSet(t)
	for i := range mset.Len() {
		if name := mset.At(i).Obj().Name(); interpretedMethod(name) {
			return name
		}
	}
	return ""
}
