package conventions_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RingZeroTestSuite holds the files that check gotest with plain Go to that
// rule: a ring-0 file calls no gotest function but the allowlisted ones.
type RingZeroTestSuite struct{}

func (s *RingZeroTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

const gotestImport = "github.com/mvrahden/go-test/pkg/gotest"

var ringZeroAllowed = []string{"DefaultSuiteConfig", "DefaultFixtureConfig", "Each", "Must"}

// The one suite that tests snapshots may take them.
const snapshotSuite = "pkg/gotest/snapshot_internals_suite_test.go"

// Directories whose every test file is ring 0.
var ringZeroDirs = []string{"pkg/gotest/internal/assert", "pkg/gotest/internal/snapfile", "tests/canary"}

// ringZeroCalls returns, per exported pkg/gotest function a ring-0 file may not
// call, how often src uses it. funcs is the set of exported package-level
// functions of pkg/gotest; rel is the file's slash path from the repository root.
func ringZeroCalls(rel string, src []byte, funcs map[string]bool) (map[string]int, error) {
	f, err := parser.ParseFile(token.NewFileSet(), rel, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	calls := map[string]int{}
	name := ""
	for _, imp := range f.Imports {
		if path, _ := strconv.Unquote(imp.Path.Value); path == gotestImport {
			name = "gotest"
			if imp.Name != nil {
				name = imp.Name.Name
			}
		}
	}
	if name == "" || name == "_" || name == "." {
		return calls, nil
	}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != name {
			return true
		}
		fn := sel.Sel.Name
		switch {
		case !funcs[fn], strings.HasPrefix(fn, "Export"), slices.Contains(ringZeroAllowed, fn):
		case fn == "MatchSnapshot" && rel == snapshotSuite:
		default:
			calls["gotest."+fn]++
		}
		return true
	})
	return calls, nil
}

// isRingZero reports whether src's package clause carries //nolint:fail-guard.
func isRingZero(src []byte) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.PackageClauseOnly)
	if err != nil {
		return false
	}
	line := fset.Position(f.Package).Line
	lines := bytes.Split(src, []byte("\n"))
	return line <= len(lines) && bytes.Contains(lines[line-1], []byte("//nolint:fail-guard"))
}

// gotestFuncs is every exported package-level function of pkg/gotest.
func gotestFuncs(t *gotest.T, root string) map[string]bool {
	dir := filepath.Join(root, "pkg", "gotest")
	entries, err := os.ReadDir(dir)
	gotest.NoError(t, err)
	funcs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.SkipObjectResolution)
		gotest.NoError(t, err)
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.IsExported() {
				funcs[fd.Name.Name] = true
			}
		}
	}
	return funcs
}

func (s *RingZeroTestSuite) TestTheScanSeesEverySpelling(t *gotest.T) {
	funcs := map[string]bool{"Equal": true, "NoError": true, "Each": true, "MatchSnapshot": true, "DefaultSuiteConfig": true}
	const head = "package x_test //nolint:fail-guard\n\nimport %s \"github.com/mvrahden/go-test/pkg/gotest\"\n\n"
	for sub, tc := range gotest.Each(t, []struct {
		Name, File, Import, Body string
		Want                     map[string]int
	}{
		{"a plain call", "a_test.go", "", "func f(t *gotest.T) { gotest.Equal(t, 1, 1) }", map[string]int{"gotest.Equal": 1}},
		{"another import name", "a_test.go", "gt", "func f(t *gt.T) { gt.NoError(t, nil) }", map[string]int{"gotest.NoError": 1}},
		{"an instantiation", "a_test.go", "", "func f(t *gotest.T) { gotest.Equal[int](t, 1, 1) }", map[string]int{"gotest.Equal": 1}},
		{"an assertion as a value", "a_test.go", "", "var check = gotest.NoError", map[string]int{"gotest.NoError": 1}},
		{"a snapshot outside its suite", "a_test.go", "", "func f(t *gotest.T) { gotest.MatchSnapshot(t, 1) }", map[string]int{"gotest.MatchSnapshot": 1}},
		{"a snapshot in its suite", snapshotSuite, "", "func f(t *gotest.T) { gotest.MatchSnapshot(t, 1) }", map[string]int{}},
		{"the allowlist", "a_test.go", "", "func f(t *gotest.T) gotest.SuiteConfig { for range gotest.Each(t, []int{1}) {}; _ = gotest.ExportAnything; return gotest.DefaultSuiteConfig() }", map[string]int{}},
		{"a type and a constant", "a_test.go", "", "var _ = gotest.SuiteConfig{Timeout: gotest.NoDeadline}", map[string]int{}},
	}) {
		got, err := ringZeroCalls(tc.File, []byte(fmt.Sprintf(head, tc.Import)+tc.Body+"\n"), funcs)
		gotest.NoError(sub, err)
		gotest.Equal(sub, tc.Want, got)
	}
	gotest.True(t, isRingZero([]byte("// doc\npackage x_test //nolint:fail-guard\n")))
	gotest.False(t, isRingZero([]byte("package x_test\n\nfunc f() {} //nolint:fail-guard\n")))
}

func (s *RingZeroTestSuite) TestRingZeroFilesCheckWithPlainGo(t *gotest.T) {
	root := filepath.Join("..", "..")
	funcs := gotestFuncs(t, root)
	gotest.NotEmpty(t, funcs)
	var found []string
	ringZeroFiles := 0
	for _, dir := range scanRoots {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), "testdata") || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				if path != filepath.Join(root, dir) {
					if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") || about.PSuiteRegex.MatchString(d.Name()) {
				return nil
			}
			src, err := os.ReadFile(path) //nolint:gosec // G122: the repository's own test files
			if err != nil || !isRingZero(src) {
				return err
			}
			ringZeroFiles++
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			calls, err := ringZeroCalls(rel, src, funcs)
			if err != nil {
				return err
			}
			for name, n := range calls {
				found = append(found, fmt.Sprintf("%s calls %s %d times: check with plain Go, if got != want { t.Errorf(...) }", rel, name, n))
			}
			return nil
		})
		gotest.NoError(t, err)
	}
	gotest.GreaterOrEqual(t, ringZeroFiles, 34)
	slices.Sort(found)
	gotest.Empty(t, found)
}

func (s *RingZeroTestSuite) TestRingZeroDirectoriesHoldOnlyRingZeroFiles(t *gotest.T) {
	root := filepath.Join("..", "..")
	var unmarked []string
	for _, dir := range ringZeroDirs {
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*_test.go"))
		gotest.NoError(t, err)
		gotest.NotEmpty(t, files, "%s holds no test file", dir)
		for _, file := range files {
			if about.PSuiteRegex.MatchString(filepath.Base(file)) {
				continue
			}
			src, err := os.ReadFile(file)
			gotest.NoError(t, err)
			if !isRingZero(src) {
				unmarked = append(unmarked, dir+"/"+filepath.Base(file)+" is in a ring-0 package: put //nolint:fail-guard on its package clause")
			}
		}
	}
	gotest.Empty(t, unmarked)
}
