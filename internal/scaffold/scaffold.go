package scaffold

import (
	"embed"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/goversion"
	"golang.org/x/tools/go/packages"
)

//go:embed static
var templates embed.FS

// TypeInfo describes a Go type extracted from a package.
type TypeInfo struct {
	Name        string
	PkgName     string
	PkgDir      string // absolute dir for output file placement
	IsInterface bool
	IsFuncBased bool // true when scaffolding standalone functions (no struct/interface)
	Methods     []MethodInfo
}

// MethodInfo describes a single method on a type.
type MethodInfo struct {
	Name         string
	Signature    string // human-readable param/return description for TODO comment
	ReturnsError bool
}

// FuncInfo describes an exported package-level function.
type FuncInfo struct {
	Name      string
	Signature string
}

// FileInfo describes a Go source file's exported functions for scaffold generation.
type FileInfo struct {
	SuiteName string
	PkgName   string
	PkgDir    string
	Funcs     []FuncInfo
}

// ParseTarget parses a target string like "./pkg/user.UserService" into
// a package pattern and type name. The separator is the last dot where
// the part after it starts with an uppercase letter.
func ParseTarget(target string) (pkgPattern, typeName string, err error) {
	if target == "" {
		return "", "", fmt.Errorf("empty target")
	}

	// Find the last dot where the next character is uppercase
	lastDot := -1
	for i := len(target) - 1; i >= 0; i-- {
		if target[i] == '.' {
			if i+1 < len(target) && unicode.IsUpper(rune(target[i+1])) {
				lastDot = i
				break
			}
		}
	}

	if lastDot == -1 {
		return "", "", fmt.Errorf("no type name found in target %q (expected format: ./pkg/path.TypeName)", target)
	}

	pkgPattern = target[:lastDot]
	typeName = target[lastDot+1:]

	if pkgPattern == "" {
		return "", "", fmt.Errorf("empty package pattern in target %q", target)
	}
	if typeName == "" {
		return "", "", fmt.Errorf("empty type name in target %q", target)
	}

	return pkgPattern, typeName, nil
}

// IntrospectType loads the package and extracts type information for the
// given type name. It works in non-test mode to access production types.
func IntrospectType(pkgPattern, typeName string) (*TypeInfo, error) {
	cfg, pattern := loadConfig(pkgPattern,
		packages.NeedName|packages.NeedTypes|packages.NeedImports|packages.NeedDeps|packages.NeedFiles|packages.NeedModule)

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		if merr := methodTarget(pkgPattern, typeName); merr != nil {
			return nil, merr
		}
		return nil, fmt.Errorf("failed to load package %q: %w", pkgPattern, err)
	}
	if err := goversion.Check(pkgs); err != nil {
		return nil, err
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for pattern %q", pkgPattern)
	}

	// Check for package errors
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			if merr := methodTarget(pkgPattern, typeName); merr != nil {
				return nil, merr
			}
			return nil, fmt.Errorf("package %q has errors: %v", pkgPattern, pkg.Errors[0])
		}
	}

	pkg := pkgs[0]
	scope := pkg.Types.Scope()
	obj := scope.Lookup(typeName)
	if obj == nil {
		return nil, fmt.Errorf("type %q not found in package %q", typeName, pkgPattern)
	}

	typeNameObj, ok := obj.(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%q is not a type in package %q", typeName, pkgPattern)
	}

	named, ok := typeNameObj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%q is not a named type", typeName)
	}

	if err := genericType(named); err != nil {
		return nil, err
	}

	info := &TypeInfo{
		Name:    typeName,
		PkgName: pkg.Name,
		PkgDir:  gotestgen.DeterminePkgDir(pkg),
	}

	// Check if interface
	underlying := named.Underlying()
	suiteName := typeName + "TestSuite"
	if _, ok := underlying.(*types.Interface); ok {
		suiteName = typeName + "ContractTestSuite"
	}
	if err := checkSuiteName(info.PkgDir, pkg.Name, suiteName); err != nil {
		return nil, err
	}
	if iface, ok := underlying.(*types.Interface); ok {
		info.IsInterface = true
		info.Methods = extractInterfaceMethods(iface)
	} else {
		info.IsInterface = false
		info.Methods = extractStructMethods(named)
	}

	if len(info.Methods) == 0 {
		return nil, fmt.Errorf("%s has no exported method: %s would declare no case, which gotest refuses, so nothing was written", typeName, suiteName)
	}
	sort.Slice(info.Methods, func(i, j int) bool {
		return info.Methods[i].Name < info.Methods[j].Name
	})

	return info, nil
}

// IntrospectFile loads the package in dir and extracts exported package-level
// functions from the named file. It returns a FileInfo suitable for file-scoped
// scaffold generation. The package is loaded from dir itself, so go resolves
// the module and workspace the file belongs to by the directory as given — a
// symlinked or short-named path included — and no path is compared outside go.
func IntrospectFile(dir, filename string) (*FileInfo, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedFiles | packages.NeedModule,
		Tests: false,
		Dir:   dir,
	}
	pkgPattern := "."

	pkgs, err := packages.Load(cfg, pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %q: %w", pkgPattern, err)
	}
	if err := goversion.Check(pkgs); err != nil {
		return nil, err
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for pattern %q", pkgPattern)
	}

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("package %q has errors: %v", pkgPattern, pkg.Errors[0])
		}
	}

	pkg := pkgs[0]

	// Find the AST file matching filename by comparing base names from pkg.GoFiles
	var astFile *ast.File
	for i, goFile := range pkg.GoFiles {
		if filepath.Base(goFile) == filename {
			astFile = pkg.Syntax[i]
			break
		}
	}
	if astFile == nil {
		return nil, fmt.Errorf("file %q not found in package %q", filename, pkgPattern)
	}

	scope := pkg.Types.Scope()
	var funcs []FuncInfo

	for _, decl := range astFile.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Recv != nil {
			continue
		}
		if !ast.IsExported(fd.Name.Name) {
			continue
		}

		obj := scope.Lookup(fd.Name.Name)
		if obj == nil {
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig := fn.Type().(*types.Signature)
		funcs = append(funcs, FuncInfo{
			Name:      fn.Name(),
			Signature: formatSignature(sig),
		})
	}

	sort.Slice(funcs, func(i, j int) bool {
		return funcs[i].Name < funcs[j].Name
	})

	base := strings.TrimSuffix(filename, ".go")
	suiteName := toPascalCase(base) + "TestSuite"
	pkgDir := gotestgen.DeterminePkgDir(pkg)
	if err := checkSuiteName(pkgDir, pkg.Name, suiteName); err != nil {
		return nil, err
	}
	if len(funcs) == 0 {
		return nil, fmt.Errorf("%s declares no exported function: %s would declare no case, which gotest refuses, so nothing was written", filename, suiteName)
	}

	return &FileInfo{
		SuiteName: suiteName,
		PkgName:   pkg.Name,
		PkgDir:    pkgDir,
		Funcs:     funcs,
	}, nil
}

// toPascalCase turns a file name into an exported identifier: every run of
// letters and digits starts upper case, anything else separates. A name that
// would start with a digit gets a File prefix.
func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var result strings.Builder
	for _, p := range parts {
		r, size := utf8.DecodeRuneInString(p)
		result.WriteRune(unicode.ToUpper(r))
		result.WriteString(p[size:])
	}
	out := result.String()
	if r, _ := utf8.DecodeRuneInString(out); out == "" || !unicode.IsUpper(r) {
		out = "File" + out
	}
	return out
}

// loadConfig returns the load config and pattern for a target's package. A
// path is loaded from its own directory, so go finds the module and workspace
// from there: from outside the module and through a symlinked directory too.
// The directory is absolute: go/packages passes it to the go command as PWD,
// which keeps a symlinked working directory's paths only when it is.
func loadConfig(pkgPattern string, mode packages.LoadMode) (*packages.Config, string) {
	cfg := &packages.Config{Mode: mode, Tests: false}
	if !isPathPattern(pkgPattern) {
		return cfg, pkgPattern
	}
	cfg.Dir = pkgPattern
	if abs, err := filepath.Abs(pkgPattern); err == nil {
		cfg.Dir = abs
	}
	return cfg, "."
}

// isPathPattern reports whether a package pattern is a directory path, not
// an import path or a wildcard.
func isPathPattern(p string) bool {
	if strings.Contains(p, "...") {
		return false
	}
	if filepath.IsAbs(p) {
		return true
	}
	slashed := filepath.ToSlash(p)
	return slashed == "." || slashed == ".." || strings.HasPrefix(slashed, "./") || strings.HasPrefix(slashed, "../")
}

// methodTarget explains a target that names a method, pkg.Type.Method, or
// returns nil when the target is not one.
func methodTarget(pkgPattern, method string) error {
	innerPattern, typeName, err := ParseTarget(pkgPattern)
	if err != nil {
		return nil
	}
	cfg, pattern := loadConfig(innerPattern, packages.NeedName|packages.NeedTypes)
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		return nil
	}
	tn, ok := pkgs[0].Types.Scope().Lookup(typeName).(*types.TypeName)
	if !ok {
		return nil
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		return nil
	}
	if obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(named), true, named.Obj().Pkg(), method); obj == nil {
		return nil
	}
	if err := genericType(named); err != nil {
		return fmt.Errorf("%s.%s is a method of a generic type: %w", typeName, method, err)
	}
	return fmt.Errorf("%s.%s is a method; scaffold its type instead: %s", typeName, method, pkgPattern)
}

// errGeneric explains why a generic target is refused: a suite must name
// the subject with type arguments, and scaffold cannot choose them.
func errGeneric(kind, name string, tparams *types.TypeParamList) error {
	params := make([]string, tparams.Len())
	for i := range params {
		tp := tparams.At(i)
		params[i] = tp.Obj().Name() + " " + types.TypeString(tp.Constraint(), shortQualifier)
	}
	return fmt.Errorf("%s is generic (%s %s[%s]): a suite needs type arguments scaffold cannot choose, so nothing was written",
		name, kind, name, strings.Join(params, ", "))
}

// genericType refuses a generic named type.
func genericType(named *types.Named) error {
	if named.TypeParams().Len() == 0 {
		return nil
	}
	return errGeneric("type", named.Obj().Name(), named.TypeParams())
}

// checkSuiteName refuses a suite name the package already declares at
// package level, in any of its files or its external test package's, build
// constraints aside: a second declaration would not compile.
func checkSuiteName(pkgDir, pkgName, suiteName string) error {
	dirEntries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", pkgDir, err)
	}
	fset := token.NewFileSet()
	for _, de := range dirEntries {
		name := de.Name()
		if de.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(pkgDir, name)
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil || (f.Name.Name != pkgName && f.Name.Name != pkgName+"_test") {
			continue
		}
		if kind, ident := packageDecl(f, suiteName); ident != nil {
			pos := fset.Position(ident.Pos())
			return fmt.Errorf("%s is already declared: %s %s in package %s at %s:%d",
				suiteName, kind, suiteName, f.Name.Name, pos.Filename, pos.Line)
		}
	}
	return nil
}

// packageDecl finds the package-level declaration of name in f.
func packageDecl(f *ast.File, name string) (kind string, ident *ast.Ident) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == name {
				return "func", d.Name
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					if sp.Name.Name == name {
						return "type", sp.Name
					}
				case *ast.ValueSpec:
					for _, id := range sp.Names {
						if id.Name == name {
							return d.Tok.String(), id
						}
					}
				}
			}
		}
	}
	return "", nil
}

func extractStructMethods(named *types.Named) []MethodInfo {
	// Get pointer receiver methods (includes value receiver methods)
	mset := types.NewMethodSet(types.NewPointer(named))
	var methods []MethodInfo
	for i := 0; i < mset.Len(); i++ {
		sel := mset.At(i)
		// Only direct methods (not promoted)
		if len(sel.Index()) != 1 {
			continue
		}
		fn, ok := sel.Obj().(*types.Func)
		if !ok {
			continue
		}
		// Only exported methods
		if !fn.Exported() {
			continue
		}
		sig := fn.Type().(*types.Signature)
		methods = append(methods, MethodInfo{
			Name:         fn.Name(),
			Signature:    formatSignature(sig),
			ReturnsError: returnsError(sig),
		})
	}
	return methods
}

func extractInterfaceMethods(iface *types.Interface) []MethodInfo {
	var methods []MethodInfo
	for i := 0; i < iface.NumMethods(); i++ {
		fn := iface.Method(i)
		if !fn.Exported() {
			continue
		}
		sig := fn.Type().(*types.Signature)
		methods = append(methods, MethodInfo{
			Name:         fn.Name(),
			Signature:    formatSignature(sig),
			ReturnsError: returnsError(sig),
		})
	}
	return methods
}

// shortQualifier strips package paths to just the package name for readability.
func shortQualifier(pkg *types.Package) string {
	return pkg.Name()
}

func formatSignature(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("(")
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		p := params.At(i)
		if p.Name() != "" {
			b.WriteString(p.Name())
			b.WriteString(" ")
		}
		b.WriteString(types.TypeString(p.Type(), shortQualifier))
	}
	b.WriteString(")")

	results := sig.Results()
	if results.Len() > 0 {
		b.WriteString(" ")
		if results.Len() == 1 {
			b.WriteString(types.TypeString(results.At(0).Type(), shortQualifier))
		} else {
			b.WriteString("(")
			for i := 0; i < results.Len(); i++ {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(types.TypeString(results.At(i).Type(), shortQualifier))
			}
			b.WriteString(")")
		}
	}

	return b.String()
}

func returnsError(sig *types.Signature) bool {
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		if results.At(i).Type().String() == "error" {
			return true
		}
	}
	return false
}

// toSnakeCase converts PascalCase to snake_case.
// Example: UserService -> user_service, HTTPClient -> http_client
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			prev := rune(s[i-1])
			if !unicode.IsUpper(prev) {
				result.WriteByte('_')
			} else if i+1 < len(s) && !unicode.IsUpper(rune(s[i+1])) {
				result.WriteByte('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// ToSnakeCase is the exported version of toSnakeCase for use by the CLI.
func ToSnakeCase(s string) string {
	return toSnakeCase(s)
}

var (
	structTemplate   = template.Must(template.New("struct").ParseFS(templates, "static/scaffold.struct.go.tpl"))
	contractTemplate = template.Must(template.New("contract").ParseFS(templates, "static/scaffold.contract.go.tpl"))
	fileTemplate     = template.Must(template.New("file").ParseFS(templates, "static/scaffold.file.go.tpl"))
)

// GenerateFileScaffold generates a test suite scaffold for package-level functions.
func GenerateFileScaffold(info *FileInfo) ([]byte, error) {
	var buf strings.Builder
	if err := fileTemplate.ExecuteTemplate(&buf, "scaffold.file.go.tpl", info); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}
	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return nil, fmt.Errorf("go/format failed: %w", err)
	}
	return formatted, nil
}

// GenerateScaffold generates a test suite scaffold for a struct type.
func GenerateScaffold(info *TypeInfo) ([]byte, error) {
	var buf strings.Builder
	if err := structTemplate.ExecuteTemplate(&buf, "scaffold.struct.go.tpl", info); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return nil, fmt.Errorf("go/format failed: %w", err)
	}

	return formatted, nil
}

// GenerateContractScaffold generates a generic contract test suite scaffold
// for an interface type.
func GenerateContractScaffold(info *TypeInfo) ([]byte, error) {
	var buf strings.Builder
	if err := contractTemplate.ExecuteTemplate(&buf, "scaffold.contract.go.tpl", info); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return nil, fmt.Errorf("go/format failed: %w", err)
	}

	return formatted, nil
}
