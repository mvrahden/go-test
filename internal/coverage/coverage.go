package coverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
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
	Name    string
	Covered bool
}

func Analyze(profilePath string, patterns []string) (*Report, error) {
	profiles, err := cover.ParseProfiles(profilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing cover profile: %w", err)
	}

	profileIndex := indexProfiles(profiles)

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
		pr := analyzePackage(pkg, profileIndex)
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
					fmt.Fprintf(w, "  ✓ %s\n", mr.Name)
				} else {
					fmt.Fprintf(w, "  ✗ %s\n", mr.Name)
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

func indexProfiles(profiles []*cover.Profile) map[string][]cover.ProfileBlock {
	index := make(map[string][]cover.ProfileBlock, len(profiles))
	for _, p := range profiles {
		index[p.FileName] = p.Blocks
	}
	return index
}

func analyzePackage(pkg *packages.Package, profileIndex map[string][]cover.ProfileBlock) PackageReport {
	pr := PackageReport{Path: pkg.PkgPath}

	prodTypes := findProductionTypes(pkg)
	if len(prodTypes) == 0 {
		return pr
	}

	methodPositions := findMethodPositions(pkg.GoFiles)

	for _, pt := range prodTypes {
		tr := TypeReport{Name: pt.name}
		for _, methodName := range pt.methods {
			mr := MethodReport{Name: methodName}
			key := pt.name + "." + methodName
			if pos, ok := methodPositions[key]; ok {
				profileKey := pkg.PkgPath + "/" + filepath.Base(pos.file)
				if blocks, ok := profileIndex[profileKey]; ok {
					mr.Covered = isBlockCovered(blocks, pos)
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

type methodPos struct {
	file      string
	startLine int
	endLine   int
}

func findMethodPositions(files []string) map[string]methodPos {
	positions := make(map[string]methodPos)
	fset := token.NewFileSet()

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Body == nil {
				continue
			}
			recvType := receiverTypeName(fd.Recv)
			if recvType == "" {
				continue
			}
			key := recvType + "." + fd.Name.Name
			positions[key] = methodPos{
				file:      file,
				startLine: fset.Position(fd.Body.Pos()).Line,
				endLine:   fset.Position(fd.Body.End()).Line,
			}
		}
	}

	return positions
}

func isBlockCovered(blocks []cover.ProfileBlock, pos methodPos) bool {
	for _, b := range blocks {
		if b.Count > 0 && b.StartLine <= pos.endLine && b.EndLine >= pos.startLine {
			return true
		}
	}
	return false
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
