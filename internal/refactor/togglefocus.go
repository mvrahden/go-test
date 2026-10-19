package refactor

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
)

// ToggleFocus adds or removes the F_ prefix of the suite or method
// identifier names ("Suite" or "Suite.Method"), declared in filePath, at
// every reference to it in the package's files, test files of both test
// packages included. Nothing is written unless every edit applies.
func ToggleFocus(filePath string, identifier string) error {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	suiteName, methodName, _ := strings.Cut(identifier, ".")
	var name *ast.Ident
	if methodName != "" {
		if name = methodIdent(file, suiteName, methodName); name == nil {
			return fmt.Errorf("method %s.%s not found in %s", suiteName, methodName, filePath)
		}
	} else if name = typeIdent(file, suiteName); name == nil {
		return fmt.Errorf("type %s not found in %s", suiteName, filePath)
	}

	offsets, err := references(filePath, fset.Position(name.Pos()).Offset)
	if err != nil {
		return err
	}
	return applyToggle(offsets, name.Name, togglePrefix(name.Name))
}

func typeIdent(file *ast.File, suiteName string) *ast.Ident {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == suiteName {
				return ts.Name
			}
		}
	}
	return nil
}

func methodIdent(file *ast.File, suiteName, methodName string) *ast.Ident {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		if gotestast.ReceiverTypeName(fd.Recv.List[0].Type) == suiteName && fd.Name.Name == methodName {
			return fd.Name
		}
	}
	return nil
}

func togglePrefix(name string) string {
	if strings.HasPrefix(name, protocol.PrefixFocused) {
		return strings.TrimPrefix(name, protocol.PrefixFocused)
	}
	return protocol.PrefixFocused + name
}

// checked is one type-check of a package's files.
type checked struct {
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
}

// references returns, by file, the offsets of every identifier referring
// to the object declared at offset in filePath, over each variant of its
// package.
func references(filePath string, offset int) (map[string][]int, error) {
	views, err := load(filePath)
	if err != nil {
		return nil, err
	}
	var target Object
	found := false
	for _, v := range views {
		if name := fileNamed(v, filePath); name != "" {
			tf := fileOf(v, name)
			if target, found = DeclaredAt(v.info, tf.Pos(offset)); found {
				break
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("no type information for the declaration at %s:%d", filePath, offset)
	}
	out := map[string][]int{}
	seen := map[string]bool{}
	for _, v := range views {
		for _, id := range Idents(v.files, v.info, target) {
			p := v.fset.Position(id.Pos())
			if key := fmt.Sprintf("%s:%d", p.Filename, p.Offset); !seen[key] {
				seen[key] = true
				out[p.Filename] = append(out[p.Filename], p.Offset)
			}
		}
	}
	return out, nil
}

func fileOf(v checked, name string) *token.File {
	for _, f := range v.files {
		if tf := v.fset.File(f.FileStart); tf.Name() == name {
			return tf
		}
	}
	return nil
}

// fileNamed returns the name v knows filePath by, or "".
func fileNamed(v checked, filePath string) string {
	want, err := os.Stat(filePath)
	if err != nil {
		return ""
	}
	for _, f := range v.files {
		name := v.fset.File(f.FileStart).Name()
		if fi, err := os.Stat(name); err == nil && os.SameFile(fi, want) {
			return name
		}
	}
	return ""
}

// load type-checks the variants of filePath's package with go/packages.
// Where that leaves the file out, outside a module for one, it type-checks
// the directory's files by package clause, without their imports.
func load(filePath string) ([]checked, error) {
	dir := filepath.Dir(filePath)
	cfg := &packages.Config{
		Dir:   dir,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: true,
	}
	var views []checked
	if pkgs, err := packages.Load(cfg, "."); err == nil {
		for _, p := range pkgs {
			if p.TypesInfo != nil {
				views = append(views, checked{p.Fset, p.Syntax, p.TypesInfo})
			}
		}
	}
	for _, v := range views {
		if fileNamed(v, filePath) != "" {
			return views, nil
		}
	}
	return checkDir(dir)
}

// checkDir type-checks dir's Go files, one package per package clause.
func checkDir(dir string) ([]checked, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	byPkg := map[string][]*ast.File{}
	for _, path := range matches {
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		byPkg[f.Name.Name] = append(byPkg[f.Name.Name], f)
	}
	var views []checked
	for _, files := range byPkg {
		info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
		conf := types.Config{
			Importer: noImports{},
			Error:    func(error) {}, // unresolved imports leave the package's own objects intact
		}
		_, _ = conf.Check(files[0].Name.Name, fset, files, info)
		views = append(views, checked{fset, files, info})
	}
	return views, nil
}

type noImports struct{}

func (noImports) Import(string) (*types.Package, error) {
	return nil, errors.New("imports are not loaded")
}

// applyToggle replaces oldName with newName at each offset and writes the
// files, once every edit is known to apply.
func applyToggle(offsets map[string][]int, oldName, newName string) error {
	contents := map[string][]byte{}
	for file, offs := range offsets {
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		sort.Sort(sort.Reverse(sort.IntSlice(offs)))
		for _, pos := range offs {
			end := pos + len(oldName)
			if end > len(src) || string(src[pos:end]) != oldName {
				return fmt.Errorf("unexpected content in %s at offset %d: expected %q", file, pos, oldName)
			}
			src = append(src[:pos], append([]byte(newName), src[end:]...)...)
		}
		contents[file] = src
	}
	for file, src := range contents {
		if err := os.WriteFile(file, src, 0o644); err != nil { //nolint:gosec // G306: not sensitive data
			return err
		}
	}
	return nil
}
