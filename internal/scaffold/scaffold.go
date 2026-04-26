package scaffold

import (
	"fmt"
	"go/format"
	"go/types"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// TypeInfo describes a Go type extracted from a package.
type TypeInfo struct {
	Name        string
	PkgName     string
	PkgDir      string // absolute dir for output file placement
	IsInterface bool
	Methods     []MethodInfo
}

// MethodInfo describes a single method on a type.
type MethodInfo struct {
	Name         string
	Signature    string // human-readable param/return description for TODO comment
	ReturnsError bool
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
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps | packages.NeedFiles,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %q: %w", pkgPattern, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for pattern %q", pkgPattern)
	}

	// Check for package errors
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
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

	info := &TypeInfo{
		Name:    typeName,
		PkgName: pkg.Name,
		PkgDir:  determinePkgDir(pkg),
	}

	// Check if interface
	underlying := named.Underlying()
	if iface, ok := underlying.(*types.Interface); ok {
		info.IsInterface = true
		info.Methods = extractInterfaceMethods(iface)
	} else {
		info.IsInterface = false
		info.Methods = extractStructMethods(named)
	}

	sort.Slice(info.Methods, func(i, j int) bool {
		return info.Methods[i].Name < info.Methods[j].Name
	})

	return info, nil
}

func determinePkgDir(pkg *packages.Package) string {
	// Use the directory from GoFiles if available
	if len(pkg.GoFiles) > 0 {
		// GoFiles contains absolute paths; extract directory from first file
		lastSlash := strings.LastIndex(pkg.GoFiles[0], "/")
		if lastSlash >= 0 {
			return pkg.GoFiles[0][:lastSlash]
		}
	}
	// Fallback: try CompiledGoFiles
	if len(pkg.CompiledGoFiles) > 0 {
		lastSlash := strings.LastIndex(pkg.CompiledGoFiles[0], "/")
		if lastSlash >= 0 {
			return pkg.CompiledGoFiles[0][:lastSlash]
		}
	}
	return ""
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

var structTemplate = template.Must(template.New("struct").Parse(`package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.Name}}TestSuite struct {
	sut *{{.Name}}
}

func (s *{{.Name}}TestSuite) BeforeEach(t *gotest.T) {
	s.sut = nil // TODO: initialize {{.Name}}
}
{{range .Methods}}
func (s *{{$.Name}}TestSuite) Test{{.Name}}(t *gotest.T) {
{{- if .ReturnsError}}
	t.It("succeeds", func(it *gotest.T) {
		// TODO: test {{$.Name}}.{{.Name}}{{.Signature}}
	})
	t.It("returns error", func(it *gotest.T) {
		// TODO: test error case
	})
{{- else}}
	t.It("works", func(it *gotest.T) {
		// TODO: test {{$.Name}}.{{.Name}}{{.Signature}}
	})
{{- end}}
}
{{end}}`))

var contractTemplate = template.Must(template.New("contract").Parse(`package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.Name}}ContractTestSuite[T {{.Name}}] struct {
	factory func() T
	sut     T
}

func (s *{{.Name}}ContractTestSuite[T]) BeforeEach(t *gotest.T) {
	s.sut = s.factory()
}
{{range .Methods}}
func (s *{{$.Name}}ContractTestSuite[T]) Test{{.Name}}(t *gotest.T) {
{{- if .ReturnsError}}
	t.It("succeeds", func(it *gotest.T) {
		// TODO: test {{$.Name}}.{{.Name}}{{.Signature}}
	})
	t.It("returns error", func(it *gotest.T) {
		// TODO: test error case
	})
{{- else}}
	t.It("works", func(it *gotest.T) {
		// TODO: test {{$.Name}}.{{.Name}}{{.Signature}}
	})
{{- end}}
}
{{end}}
// Instantiate the contract for a concrete implementation:
// type My{{.Name}}TestSuite = {{.Name}}ContractTestSuite[*MyImpl]
`))

// GenerateScaffold generates a test suite scaffold for a struct type.
func GenerateScaffold(info *TypeInfo) ([]byte, error) {
	var buf strings.Builder
	if err := structTemplate.Execute(&buf, info); err != nil {
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
	if err := contractTemplate.Execute(&buf, info); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return nil, fmt.Errorf("go/format failed: %w", err)
	}

	return formatted, nil
}
