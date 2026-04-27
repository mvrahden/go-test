package coverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Report struct {
	Packages []PackageReport
	Total    int
	Covered  int
}

type PackageReport struct {
	Path  string
	Types []TypeReport
}

type TypeReport struct {
	Name    string
	Methods []MethodReport
}

type MethodReport struct {
	Name       string
	Covered    bool
	TestMethod string
}

func Analyze(patterns []string) (*Report, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedFiles,
	}, patterns...)
	if err != nil {
		return nil, err
	}

	report := &Report{}
	for _, pkg := range pkgs {
		if len(pkg.GoFiles) == 0 {
			continue
		}
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("package %s: %v", pkg.PkgPath, pkg.Errors[0])
		}
		pr := analyzePackage(pkg)
		if len(pr.Types) > 0 {
			report.Packages = append(report.Packages, pr)
		}
	}

	for _, pr := range report.Packages {
		for _, tr := range pr.Types {
			for _, mr := range tr.Methods {
				report.Total++
				if mr.Covered {
					report.Covered++
				}
			}
		}
	}

	return report, nil
}

func Render(w io.Writer, report *Report) {
	for i, pr := range report.Packages {
		if len(report.Packages) > 1 {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "=== %s ===\n\n", pr.Path)
		}
		for _, tr := range pr.Types {
			covered, total := 0, len(tr.Methods)
			for _, mr := range tr.Methods {
				if mr.Covered {
					covered++
				}
			}
			pct := 0
			if total > 0 {
				pct = covered * 100 / total
			}
			fmt.Fprintf(w, "%s: %d/%d methods covered (%d%%)\n", tr.Name, covered, total, pct)
			for _, mr := range tr.Methods {
				if mr.Covered {
					fmt.Fprintf(w, "  ✓ %-24s — %s\n", mr.Name, mr.TestMethod)
				} else {
					fmt.Fprintf(w, "  ✗ %-24s — no test case\n", mr.Name)
				}
			}
			fmt.Fprintln(w)
		}
	}

	if report.Total > 0 {
		pct := report.Covered * 100 / report.Total
		fmt.Fprintf(w, "Overall: %d/%d methods covered (%d%%)\n", report.Covered, report.Total, pct)
	}
}

func analyzePackage(pkg *packages.Package) PackageReport {
	pr := PackageReport{Path: pkg.PkgPath}

	prodTypes := findProductionTypes(pkg)
	dir := filepath.Dir(pkg.GoFiles[0])
	suites := findTestSuites(dir)

	for _, pt := range prodTypes {
		tr := TypeReport{Name: pt.name}
		matching := findMatchingSuite(pt.name, suites)
		for _, methodName := range pt.methods {
			mr := MethodReport{Name: methodName}
			if matching != nil {
				if tm := findMatchingTestMethod(methodName, matching.methods); tm != "" {
					mr.Covered = true
					mr.TestMethod = tm
				}
			}
			tr.Methods = append(tr.Methods, mr)
		}
		pr.Types = append(pr.Types, tr)
	}

	return pr
}

type prodType struct {
	name    string
	methods []string
}

func findProductionTypes(pkg *packages.Package) []prodType {
	scope := pkg.Types.Scope()
	names := scope.Names()
	sort.Strings(names)

	var result []prodType
	for _, name := range names {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		ptrType := types.NewPointer(tn.Type())
		mset := types.NewMethodSet(ptrType)

		var methods []string
		for i := 0; i < mset.Len(); i++ {
			sel := mset.At(i)
			if !sel.Obj().Exported() {
				continue
			}
			if len(sel.Index()) > 1 {
				continue
			}
			methods = append(methods, sel.Obj().Name())
		}

		if len(methods) == 0 {
			continue
		}

		sort.Strings(methods)
		result = append(result, prodType{name: name, methods: methods})
	}

	return result
}

type testSuite struct {
	name     string
	typeName string
	methods  []string
}

func findTestSuites(dir string) []testSuite {
	fset := token.NewFileSet()
	filter := func(fi fs.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), "_test.go")
	}

	pkgMap, err := parser.ParseDir(fset, dir, filter, 0)
	if err != nil {
		return nil
	}

	suiteTypes := make(map[string]*testSuite)

	for _, astPkg := range pkgMap {
		for _, file := range astPkg.Files {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					name := ts.Name.Name
					typeName := extractTypeName(name)
					if typeName != "" {
						suiteTypes[name] = &testSuite{
							name:     name,
							typeName: typeName,
						}
					}
				}
			}

			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil {
					continue
				}
				recvName := receiverTypeName(fd.Recv)
				if suite, exists := suiteTypes[recvName]; exists {
					methodName := fd.Name.Name
					if strings.HasPrefix(methodName, "Test") {
						suite.methods = append(suite.methods, methodName)
					}
				}
			}
		}
	}

	var result []testSuite
	for _, s := range suiteTypes {
		result = append(result, *s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func extractTypeName(suiteName string) string {
	name := strings.TrimPrefix(suiteName, "F_")
	name = strings.TrimPrefix(name, "X_")
	if strings.HasSuffix(name, "TestSuiteParallel") {
		return strings.TrimSuffix(name, "TestSuiteParallel")
	}
	if strings.HasSuffix(name, "TestSuite") {
		return strings.TrimSuffix(name, "TestSuite")
	}
	return ""
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func findMatchingSuite(typeName string, suites []testSuite) *testSuite {
	for i := range suites {
		if suites[i].typeName == typeName {
			return &suites[i]
		}
	}
	return nil
}

func findMatchingTestMethod(methodName string, testMethods []string) string {
	for _, tm := range testMethods {
		stripped := strings.TrimPrefix(strings.TrimPrefix(tm, "F_"), "X_")
		if strings.TrimPrefix(stripped, "TestParallel") == methodName ||
			strings.TrimPrefix(stripped, "Test") == methodName {
			return tm
		}
	}
	return ""
}
