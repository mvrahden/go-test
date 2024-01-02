package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"

	"golang.org/x/tools/go/packages"
)

var (
	IS_TEST_SUITE          = regexp.MustCompile(`^([X|F]_)?.+TestSuite$`)
	IS_TEST_HARNESS_METHOD = regexp.MustCompile(`^(BeforeAll|AfterAll|BeforeEach|AfterEach|Test.+|TestParallel.+)$`)
	IS_BEFORE_ALL          = regexp.MustCompile(`^BeforeAll$`)
	IS_AFTER_ALL           = regexp.MustCompile(`^AfterAll$`)
	IS_BEFORE_EACH         = regexp.MustCompile(`^BeforeEach$`)
	IS_AFTER_EACH          = regexp.MustCompile(`^AfterEach$`)
	IS_TEST_CASE           = regexp.MustCompile(`^([X|F]_)?Test.+`)
	IS_TEST_CASE_PARALLEL  = regexp.MustCompile(`^([X|F]_)?TestParallel.+`)
)

type TestSuiteSpec struct {
	n   ast.Node
	ts  *ast.TypeSpec
	typ *types.Struct
	th  *TestHarness
}

type TestHarness struct {
	BeforeAll         *TestHarnessMethod
	BeforeEach        *TestHarnessMethod
	AfterAll          *TestHarnessMethod
	AfterEach         *TestHarnessMethod
	TestCases         []*TestHarnessMethod
	TestCasesParallel []*TestHarnessMethod
}

type TestHarnessMethod struct {
	n ast.Node
}

func DetermineTestSuite(n ast.Node, pkg *packages.Package) (*TestSuiteSpec, token.Pos, error) {
	decl, ok := n.(*ast.GenDecl)
	if !ok || decl.Tok != token.TYPE {
		// we only care about type declarations
		return nil, -1, nil
	}

	// assert underlying enum type
	if len(decl.Specs) != 1 {
		// hint: we don't want blocks of type declarations for test suites
		return nil, -1, nil // not a test suite
	}
	ts, ok := decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		return nil, -1, nil // not a test suite
	}

	if !IS_TEST_SUITE.MatchString(ts.Name.Name) {
		return nil, -1, nil // not a test suite
	}

	typ, ok := pkg.TypesInfo.TypeOf(ts.Type).(*types.Struct)
	if !ok {
		return nil, -1, nil
	}

	return &TestSuiteSpec{n, ts, typ, &TestHarness{}}, -1, nil
}

func DetermineTestHarness(n ast.Node, pkg *packages.Package, s *TestSuiteSpec) (token.Pos, error) {
	decl, ok := n.(*ast.FuncDecl)
	if !ok {
		// we only care about method declarations
		return -1, nil
	}
	if !decl.Name.IsExported() {
		return -1, nil
	}
	m, ok := pkg.TypesInfo.ObjectOf(decl.Name).(*types.Func)
	if !ok {
		return decl.Name.Pos(), fmt.Errorf("no signature found for method %s", decl.Name)
	}
	if !IS_TEST_HARNESS_METHOD.MatchString(m.Name()) {
		return -1, nil
	}

	sig, ok := pkg.TypesInfo.TypeOf(decl.Name).(*types.Signature)
	if !ok {
		return decl.Name.Pos(), fmt.Errorf("no signature found for method %s", decl.Name)
	}
	recv := sig.Recv()
	if recv == nil {
		return -1, nil
	}
	{ // test for non-pointer suite receivers.
		// usage of value type receivers for test-suites is discouraged.
		namedRecvr, ok := recv.Type().(*types.Named)
		if ok {
			str := namedRecvr.Obj().Name()
			if IS_TEST_SUITE.MatchString(str) { // no receiver
				return decl.Name.Pos(), fmt.Errorf("signature of %q has unsupported value type receiver", decl.Name)
			}
			return -1, nil
		}
	}
	recvPtr, ok := recv.Type().(*types.Pointer)
	if !ok {
		return -1, nil
	}

	recvType := recvPtr.Elem().(*types.Named)
	if recvType == nil || recvType.Obj().Name() != s.ts.Name.Name {
		return decl.Name.Pos(), nil // no receiver
	}

	switch {
	case IS_BEFORE_ALL.MatchString(m.Name()):
		s.th.BeforeAll = &TestHarnessMethod{n: n}
	case IS_AFTER_ALL.MatchString(m.Name()):
		s.th.AfterAll = &TestHarnessMethod{n: n}
	case IS_BEFORE_EACH.MatchString(m.Name()):
		s.th.BeforeEach = &TestHarnessMethod{n: n}
	case IS_AFTER_EACH.MatchString(m.Name()):
		s.th.AfterEach = &TestHarnessMethod{n: n}
	case IS_TEST_CASE.MatchString(m.Name()):
		if IS_TEST_CASE_PARALLEL.MatchString(m.Name()) {
			s.th.TestCasesParallel = append(s.th.TestCasesParallel, &TestHarnessMethod{n: n})
			break
		}
		s.th.TestCases = append(s.th.TestCases, &TestHarnessMethod{n: n})
	default:
		return decl.Name.Pos(), fmt.Errorf("detected unhandled test harness method %q", m.Name())
	}

	return -1, nil
}
