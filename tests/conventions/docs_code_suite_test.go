package conventions_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DocsCodeTestSuite type-checks the Go blocks of README.md and AGENTS.md
// against the real pkg/gotest, so a name or call shape the package does not
// have fails with the document's line.
type DocsCodeTestSuite struct {
	gotest *types.Package // read-only after BeforeAll
	imp    types.Importer // read-only after BeforeAll
}

func (s *DocsCodeTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute, SetupTimeout: 2 * time.Minute}
}

// docImports maps the identifier a block names a standard package by to its path.
var docImports = map[string]string{
	"context": "context", "time": "time", "os": "os", "fmt": "fmt", "errors": "errors",
	"strings": "strings", "testing": "testing", "regexp": "regexp", "io": "io", "cmp": "cmp",
	"sync": "sync", "sql": "database/sql", "json": "encoding/json", "http": "net/http", "filepath": "path/filepath",
}

// importerFunc adapts a lookup to types.Importer.
type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func (s *DocsCodeTestSuite) BeforeAll(t *gotest.T) {
	// The runtime packages are named by the vendoring example's imports.
	patterns := []string{gotestImport, gotestImport + "runtime", gotestImport + "fuzz"}
	for _, path := range docImports {
		patterns = append(patterns, path)
	}
	cfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
		Dir:     filepath.Join("..", ".."),
		Context: t.Context(),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	gotest.NoError(t, err)
	loaded := map[string]*types.Package{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Types != nil {
			loaded[p.PkgPath] = p.Types
		}
	})
	s.gotest = loaded[gotestImport]
	gotest.NotZero(t, s.gotest, "pkg/gotest did not load")
	s.imp = importerFunc(func(path string) (*types.Package, error) {
		if path == "unsafe" {
			return types.Unsafe, nil
		}
		if p, ok := loaded[path]; ok {
			return p, nil
		}
		return nil, errors.New("not loaded: " + path)
	})
}

// AfterAll has nothing to release: the loaded types live in memory.
func (s *DocsCodeTestSuite) AfterAll(t *gotest.T) {}

// docBlock is one fenced Go block: its first line's number in the document and its text.
type docBlock struct {
	line int
	text string
}

func goBlocks(markdown string) []docBlock {
	var blocks []docBlock
	var open *docBlock
	var body []string
	for i, line := range strings.Split(markdown, "\n") {
		switch {
		case open == nil && line == "```go":
			open, body = &docBlock{line: i + 2}, nil
		case open != nil && strings.TrimSpace(line) == "```":
			open.text = strings.Join(body, "\n")
			blocks = append(blocks, *open)
			open = nil
		case open != nil:
			body = append(body, line)
		}
	}
	return blocks
}

// tolerated are the errors a documentation fragment makes by naming what
// it does not declare. "(unknown type)" is the follow-on of such a name: a
// call over an undefined value cannot be checked any further.
var tolerated = regexp.MustCompile(`^undefined: \w+$|declared and not used|imported and not used|^missing return|\(unknown type\)`)

const statementsHead = "func _(t *gotest.T, b *gotest.B, f *gotest.F, poll *gotest.R, ctx context.Context) {\n"

// checkDocBlocks returns one "<name>:<line>: <problem>" per problem, in document order.
func (s *DocsCodeTestSuite) checkDocBlocks(name, markdown string) []string {
	var problems []string
	for _, block := range goBlocks(markdown) {
		problems = append(problems, s.checkBlock(name, block)...)
	}
	return problems
}

func (s *DocsCodeTestSuite) checkBlock(name string, block docBlock) []string {
	parses := func(src string) bool {
		_, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
		return err == nil
	}
	// The imports a fragment's wrapper needs: gotest, and each standard
	// package the block names.
	imports := func(always ...string) string {
		var b strings.Builder
		b.WriteString("import (\n")
		seen := map[string]bool{}
		for _, path := range always {
			seen[path] = true
			fmt.Fprintf(&b, "\t%q\n", path)
		}
		for id, path := range docImports {
			if !seen[path] && regexp.MustCompile(`\b`+id+`\.`).MatchString(block.text) {
				seen[path] = true
				if id == filepath.Base(path) {
					fmt.Fprintf(&b, "\t%q\n", path)
				} else {
					fmt.Fprintf(&b, "\t%s %q\n", id, path)
				}
			}
		}
		b.WriteString(")\n")
		return b.String()
	}
	switch {
	case parses(block.text):
		return s.typeCheck(name, block, block.text, 0)
	case parses("package docs\n" + block.text):
		head := "package docs\n" + imports(gotestImport)
		return s.typeCheck(name, block, head+block.text, strings.Count(head, "\n"))
	case parses("package docs\n" + statementsHead + block.text + "\n}\n"):
		head := "package docs\n" + imports(gotestImport, "context") + statementsHead
		return s.typeCheck(name, block, head+block.text+"\n}\n", strings.Count(head, "\n"))
	}
	return s.checkListing(name, block)
}

// typeCheck checks src, whose first offset lines precede the block.
func (s *DocsCodeTestSuite) typeCheck(name string, block docBlock, src string, offset int) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "doc.go", src, parser.SkipObjectResolution)
	if err != nil {
		return []string{fmt.Sprintf("%s:%d: %v", name, block.line, err)}
	}
	type problem struct {
		line int
		msg  string
	}
	var found []problem
	cfg := types.Config{Importer: s.imp, Error: func(err error) {
		var terr types.Error
		if !errors.As(err, &terr) || tolerated.MatchString(terr.Msg) {
			return
		}
		line := block.line + fset.Position(terr.Pos).Line - 1 - offset
		found = append(found, problem{line, terr.Msg})
	}}
	_, _ = cfg.Check("docs", fset, []*ast.File{f}, nil)
	sort.SliceStable(found, func(i, j int) bool { return found[i].line < found[j].line })
	var out []string
	for _, p := range found {
		out = append(out, fmt.Sprintf("%s:%d: %s", name, p.line, p.msg))
	}
	return out
}

var (
	listedFunc   = regexp.MustCompile(`\bgotest\.([A-Z]\w*)`)
	listedMethod = regexp.MustCompile(`\b([tbf])\.([A-Z]\w*)`)
)

// checkListing holds a list of signatures to the names pkg/gotest has.
func (s *DocsCodeTestSuite) checkListing(name string, block docBlock) []string {
	types := map[string]string{"t": "T", "b": "B", "f": "F"}
	var out []string
	for i, line := range strings.Split(block.text, "\n") {
		for _, m := range listedFunc.FindAllStringSubmatch(line, -1) {
			if obj := s.gotest.Scope().Lookup(m[1]); obj == nil || !obj.Exported() {
				out = append(out, fmt.Sprintf("%s:%d: gotest.%s is not exported by pkg/gotest", name, block.line+i, m[1]))
			}
		}
		for _, m := range listedMethod.FindAllStringSubmatchIndex(line, -1) {
			// gotest.T and the like are the type, not a call on t.
			if m[0] > 0 && line[m[0]-1] == '.' {
				continue
			}
			recv, method := line[m[2]:m[3]], line[m[4]:m[5]]
			if !s.hasMethod(types[recv], method) {
				out = append(out, fmt.Sprintf("%s:%d: %s.%s is not a method of *gotest.%s", name, block.line+i, recv, method, types[recv]))
			}
		}
	}
	return out
}

func (s *DocsCodeTestSuite) hasMethod(typeName, method string) bool {
	obj := s.gotest.Scope().Lookup(typeName)
	if obj == nil {
		return false
	}
	sel, _, _ := types.LookupFieldOrMethod(types.NewPointer(obj.Type()), true, s.gotest, method)
	_, ok := sel.(*types.Func)
	return ok
}

const fence = "```"

func doc(blocks ...string) string {
	var b strings.Builder
	for _, block := range blocks {
		b.WriteString("Prose.\n\n" + fence + "go\n" + block + "\n" + fence + "\n\n")
	}
	return b.String()
}

func (s *DocsCodeTestSuite) TestProblemsAreFoundAndFragmentsAreNot(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name, Block string
		Want        []string
	}{
		{"a fragment naming what it does not declare",
			"func (s *MyTestSuite) BeforeEach(t *gotest.T) *testCtx {\n\tdb := setupTestDB()\n\treturn &testCtx{db: db, pool: pgxpool.New()}\n}", nil},
		{"statements over undeclared values",
			"gotest.Eventually(t, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) {\n\tgotest.Equal(poll, \"ready\", getStatus())\n})", nil},
		{"a call over an undeclared package",
			"conn := gotest.Must(db.Connect(ctx))\n_ = conn", nil},
		{"an assertion that does not exist",
			"func (s *MyTestSuite) TestA(t *gotest.T) {\n\tgotest.HasPrefix(t, \"a\", \"ab\")\n}",
			[]string{"x.md:5: undefined: gotest.HasPrefix"}},
		{"a method T does not have",
			"func (s *MyTestSuite) TestA(t *gotest.T) {\n\t_ = t.Contex()\n}",
			[]string{"x.md:5: t.Contex undefined (type *gotest.T has no field or method Contex"}},
		{"too few arguments",
			"gotest.Equal(t, 1)",
			[]string{"x.md:4: not enough arguments in call to gotest.Equal"}},
		{"a listing naming what exists",
			"gotest.Equal[V any](t, expected, actual V)   // deep equality\nt.Context() context.Context\nb.Loop() bool", nil},
		{"a listing naming what does not",
			"gotest.HasSuffix(t, suffix, str string)\nt.Deadline() time.Time",
			[]string{"x.md:4: gotest.HasSuffix is not exported by pkg/gotest", "x.md:5: t.Deadline is not a method of *gotest.T"}},
	}) {
		got := s.checkDocBlocks("x.md", doc(tc.Block))
		gotest.Len(sub, got, len(tc.Want), "problems: %v", got)
		for i, want := range tc.Want {
			gotest.Contains(sub, got[i], want)
		}
	}
}

func (s *DocsCodeTestSuite) TestBlocksAreFoundWithTheirLines(t *gotest.T) {
	blocks := goBlocks("a\n" + fence + "go\nx := 1\n" + fence + "\n" + fence + "sh\nls\n" + fence + "\n" + fence + "go\ny := 2\n" + fence + "\n")
	gotest.Equal(t, []docBlock{{3, "x := 1"}, {9, "y := 2"}}, blocks)
}

func (s *DocsCodeTestSuite) TestTheDocumentsTypeCheck(t *gotest.T) {
	// AGENTS.md points at the skill for signatures and holds no Go block; a
	// block added to it is checked all the same.
	for sub, tc := range gotest.Each(t, []struct {
		Name      string
		MinBlocks int
	}{{"README.md", 20}, {"AGENTS.md", 0}}) {
		data, err := os.ReadFile(filepath.Join("..", "..", tc.Name))
		gotest.NoError(sub, err)
		gotest.GreaterOrEqual(sub, len(goBlocks(string(data))), tc.MinBlocks, "%s lost its Go blocks", tc.Name)
		gotest.Empty(sub, s.checkDocBlocks(tc.Name, string(data)))
	}
}
