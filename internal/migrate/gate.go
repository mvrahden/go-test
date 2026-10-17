package migrate

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
)

// gate keeps each migrated suite behind the build constraint its dropped
// runner had: gotest's runner follows the suite's own file, so the
// constraint moves there when the suite lives in that one file and nothing
// else in it would be gated along. Otherwise the suite is noted for a
// marker. A suite no suite.Run runs is noted too: gotest runs every suite.
func (u *unit) gate() {
	runners := map[*suiteDecl][]*unitFile{}
	ran := map[*suiteDecl]bool{}
	parts := map[*suiteDecl]map[*unitFile]bool{}
	for _, uf := range u.files {
		if uf.refused || uf.skipped() {
			continue
		}
		for _, decl := range uf.f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if s := u.runnerSuite(fd); s != nil {
				runners[s] = append(runners[s], uf)
			}
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				if s := u.suiteOf(fd.Recv.List[0].Type); s != nil {
					addPart(parts, s, uf)
				}
			}
		}
		ast.Inspect(uf.f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && u.isSuiteRun(call) {
				if s := u.suiteOfType(u.info.TypeOf(call.Args[1])); s != nil {
					ran[s] = true
				}
			}
			return true
		})
	}

	for _, s := range u.suites {
		if !s.migrated() {
			continue
		}
		addPart(parts, s, s.file)
		if !ran[s] {
			s.note = "no suite.Run ran " + s.obj.Name() + ", so testify never did; gotest runs every suite — prefix it with X_ to keep it off"
			continue
		}
		gates := map[string]bool{}
		for _, uf := range runners[s] {
			gates[constraintOf(uf)] = true
		}
		if len(runners[s]) == 0 || gates[""] {
			continue // run by the default build, or by a runner that stays
		}
		names := make([]string, 0, len(gates))
		for g := range gates {
			names = append(names, g)
		}
		sort.Strings(names)
		var home *unitFile
		for uf := range parts[s] {
			home = uf
		}
		switch {
		case len(names) == 1 && len(parts[s]) == 1 && constraintOf(home) == names[0]:
		case len(names) == 1 && len(parts[s]) == 1 && constraintOf(home) == "" && u.gatesOnly(home, s):
			home.gate = names[0]
		default:
			files := make([]string, 0, len(runners[s]))
			for _, uf := range runners[s] {
				files = append(files, filepath.Base(uf.path))
			}
			s.note = s.obj.Name() + "'s runner in " + strings.Join(files, ", ") + " built only with //go:build " + strings.Join(names, " or ") +
				"; gotest runs a suite under its own file's constraint — move the suite and its methods into one file with that constraint"
		}
	}
}

func addPart(parts map[*suiteDecl]map[*unitFile]bool, s *suiteDecl, uf *unitFile) {
	if parts[s] == nil {
		parts[s] = map[*unitFile]bool{}
	}
	parts[s][uf] = true
}

// constraintOf is the //go:build expression of uf, empty for none.
func constraintOf(uf *unitFile) string {
	if expr := headerConstraint(uf.src); expr != nil {
		return expr.String()
	}
	return ""
}

// gatesOnly reports whether a constraint on uf would gate s alone: uf
// declares no other suite and no test function.
func (u *unit) gatesOnly(uf *unitFile, s *suiteDecl) bool {
	for _, other := range u.suites {
		if other != s && other.file == uf {
			return false
		}
	}
	for _, decl := range uf.f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
				if strings.HasPrefix(d.Name.Name, prefix) && u.runnerSuite(d) == nil {
					return false
				}
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name != s.obj.Name() && strings.HasSuffix(ts.Name.Name, "TestSuite") {
					if _, ok := u.info.Defs[ts.Name].(*types.TypeName); ok {
						return false
					}
				}
			}
		}
	}
	return true
}
