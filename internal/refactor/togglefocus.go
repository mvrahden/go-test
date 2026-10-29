package refactor

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
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

	newName := togglePrefix(name.Name)
	offsets, err := references(filePath, fset.Position(name.Pos()).Offset, newName)
	if err != nil {
		return err
	}
	return applyToggle(offsets, name.Name, newName)
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
	pkg   *types.Package
}

// references returns, by file, the offsets of every identifier referring
// to the object declared at offset in filePath, over each variant of its
// package, the files a build constraint leaves out included. It refuses a
// rename to newName that would not compile.
func references(filePath string, offset int, newName string) (map[string][]int, error) {
	views, unchecked, err := load(filePath)
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
	if err := refuse(views, unchecked, target, newName); err != nil {
		return nil, err
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
// A file a build constraint leaves out is type-checked in a load with the
// tags its constraint names; one no such load type-checks is returned
// parsed, in unchecked. Where go/packages leaves filePath out, outside a
// module for one, it type-checks the directory's files by package clause,
// without their imports.
func load(filePath string) (views []checked, unchecked []parsedFile, err error) {
	dir := filepath.Dir(filePath)
	pkgs := loadTagged(dir, "")
	for _, p := range pkgs {
		if p.TypesInfo != nil {
			views = append(views, checked{p.Fset, p.Syntax, p.TypesInfo, p.Types})
		}
	}
	if !slices.ContainsFunc(views, func(v checked) bool { return fileNamed(v, filePath) != "" }) {
		views, err := checkDir(dir)
		return views, nil, err
	}
	byTags := map[string][]string{}
	for _, file := range ignoredFiles(pkgs, dir) {
		tags, err := constraintTags(file)
		if err != nil {
			return nil, nil, err
		}
		byTags[tags] = append(byTags[tags], file)
	}
	for tags, files := range byTags {
		if tags != "" {
			for _, p := range loadTagged(dir, tags) {
				if p.TypesInfo == nil || len(p.Errors) > 0 {
					continue
				}
				v := checked{p.Fset, p.Syntax, p.TypesInfo, p.Types}
				files = slices.DeleteFunc(files, func(f string) bool { return fileNamed(v, f) != "" })
				views = append(views, v)
			}
		}
		for _, f := range files {
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, nil, fmt.Errorf("parsing %s: %w", f, err)
			}
			unchecked = append(unchecked, parsedFile{fset, parsed})
		}
	}
	return views, unchecked, nil
}

// loadTagged loads dir's package and its test variants under tags.
func loadTagged(dir, tags string) []*packages.Package {
	cfg := &packages.Config{
		Dir:   dir,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: true,
	}
	if tags != "" {
		cfg.BuildFlags = []string{"-tags=" + tags}
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil
	}
	return pkgs
}

// ignoredFiles lists the Go files in dir the loads in pkgs left out.
func ignoredFiles(pkgs []*packages.Package, dir string) []string {
	want, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range pkgs {
		for _, f := range p.IgnoredFiles {
			if !strings.HasSuffix(f, ".go") || slices.Contains(out, f) {
				continue
			}
			if fi, err := os.Stat(filepath.Dir(f)); err == nil && os.SameFile(fi, want) {
				out = append(out, f)
			}
		}
	}
	sort.Strings(out)
	return out
}

// constraintTags names, comma-separated and sorted, the tags file's
// //go:build line mentions; empty when it has none.
func constraintTags(file string) (string, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	f, err := parser.ParseFile(token.NewFileSet(), file, src, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return "", nil //nolint:nilerr // a file that does not parse is no file go builds
	}
	var tags []string
	for _, group := range f.Comments {
		if group.Pos() > f.Package {
			break
		}
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				continue
			}
			expr.Eval(func(tag string) bool {
				if !slices.Contains(tags, tag) {
					tags = append(tags, tag)
				}
				return true
			})
		}
	}
	sort.Strings(tags)
	return strings.Join(tags, ","), nil
}

// refuse returns why renaming target to newName would leave the package
// broken: the new name is declared already, an interface in the package
// names the method, or a file no load type-checks spells the old name.
func refuse(views []checked, unchecked []parsedFile, target Object, newName string) error {
	for _, v := range views {
		if v.pkg == nil || v.pkg.Path() != target.PkgPath {
			continue
		}
		if obj := declared(v.pkg, target, newName); obj != nil {
			return fmt.Errorf("%s: %s is already declared; toggling focus would declare it twice", v.fset.Position(obj.Pos()), newName)
		}
	}
	if target.Method != "" {
		for _, v := range views {
			for _, f := range v.files {
				if pos := interfaceNaming(f, target.Method); pos.IsValid() {
					return fmt.Errorf("%s: an interface names %s; renaming the method would change which types implement it", v.fset.Position(pos), target.Method)
				}
			}
		}
	}
	for _, f := range unchecked {
		var at *ast.Ident
		ast.Inspect(f.file, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == target.Name() {
				at = id
			}
			return at == nil
		})
		if at != nil {
			return fmt.Errorf("%s: %s cannot be resolved: no build of the package type-checks this file; rename it by hand", f.fset.Position(at.Pos()), target.Name())
		}
	}
	return nil
}

// parsedFile is a file parsed without type-checking it.
type parsedFile struct {
	fset *token.FileSet
	file *ast.File
}

// declared returns what already holds newName where target would take it:
// the package scope for a type, the type's fields and methods for a method.
func declared(pkg *types.Package, target Object, newName string) types.Object {
	if target.Method == "" {
		return pkg.Scope().Lookup(newName)
	}
	tn, ok := pkg.Scope().Lookup(target.Type).(*types.TypeName)
	if !ok {
		return nil
	}
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(tn.Type()), true, pkg, newName)
	return obj
}

// interfaceNaming returns the position of an interface type in f that
// declares a method named method, or token.NoPos.
func interfaceNaming(f *ast.File, method string) token.Pos {
	pos := token.NoPos
	ast.Inspect(f, func(n ast.Node) bool {
		it, ok := n.(*ast.InterfaceType)
		if !ok || pos.IsValid() {
			return !pos.IsValid()
		}
		for _, m := range it.Methods.List {
			for _, name := range m.Names {
				if name.Name == method {
					pos = it.Pos()
				}
			}
		}
		return true
	})
	return pos
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
		pkg, _ := conf.Check(files[0].Name.Name, fset, files, info)
		views = append(views, checked{fset, files, info, pkg})
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
