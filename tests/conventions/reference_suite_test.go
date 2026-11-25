package conventions_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReferenceTestSuite holds the hand-written reference lists to the code: the
// skill lists every assertion once with its signature, the README's tour and
// the website name every assertion, the config fields are named where they
// are tabled, and the design spec tables every environment variable a user
// may set. The lists keep their own groups, order and wording.
type ReferenceTestSuite struct{}

func (s *ReferenceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

var repoRoot = filepath.Join("..", "..")

func readDoc(t *gotest.T, rel string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel))) //nolint:gosec // G304: a document of this repository
	gotest.NoError(t, err)
	return string(data)
}

// signature is an assertion's type parameters, parameter types and names
// after t, and results, each as source prints them; msgAndArgs is left out.
type signature struct{ tparams, params, names, results string }

func (g signature) String() string {
	return fmt.Sprintf("[%s](%s | %s) %s", g.tparams, g.params, g.names, g.results)
}

func printed(fset *token.FileSet, e ast.Expr) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, e)
	return b.String()
}

func signatureOf(fset *token.FileSet, ft *ast.FuncType) signature {
	var tparams, params, names, results []string
	if ft.TypeParams != nil {
		for _, f := range ft.TypeParams.List {
			for _, n := range f.Names {
				tparams = append(tparams, n.Name+" "+printed(fset, f.Type))
			}
		}
	}
	list := ft.Params.List[1:]
	if n := len(list); n > 0 && len(list[n-1].Names) == 1 && list[n-1].Names[0].Name == "msgAndArgs" {
		list = list[:n-1]
	}
	for _, f := range list {
		for _, n := range f.Names {
			params = append(params, printed(fset, f.Type))
			names = append(names, n.Name)
		}
	}
	if ft.Results != nil {
		for _, f := range ft.Results.List {
			for range max(len(f.Names), 1) {
				results = append(results, printed(fset, f.Type))
			}
		}
	}
	return signature{strings.Join(tparams, ", "), strings.Join(params, ", "), strings.Join(names, ", "), strings.Join(results, ", ")}
}

// gotestSource parses the non-test files of pkg/gotest.
func gotestSource(t *gotest.T) (*token.FileSet, []*ast.File) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(repoRoot, "pkg", "gotest", "*.go"))
	gotest.NoError(t, err)
	var files []*ast.File
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		gotest.NoError(t, err)
		files = append(files, f)
	}
	return fset, files
}

// assertions are the exported functions of pkg/gotest whose first parameter
// is its testingT, by name.
func assertions(t *gotest.T) map[string]signature {
	fset, files := gotestSource(t)
	out := map[string]signature{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || len(fn.Type.Params.List) == 0 {
				continue
			}
			if id, ok := fn.Type.Params.List[0].Type.(*ast.Ident); ok && id.Name == "testingT" {
				out[fn.Name.Name] = signatureOf(fset, fn.Type)
			}
		}
	}
	gotest.NotEmpty(t, out, "pkg/gotest has no assertion")
	return out
}

// configFields are the exported fields of the named struct type of pkg/gotest.
func configFields(t *gotest.T, typeName string) []string {
	_, files := gotestSource(t)
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok || ts.Name.Name != typeName {
					continue
				}
				var names []string
				for _, fd := range st.Fields.List {
					for _, n := range fd.Names {
						if n.IsExported() {
							names = append(names, n.Name)
						}
					}
				}
				return names
			}
		}
	}
	gotest.Fail(t, "pkg/gotest declares no struct %s", typeName)
	return nil
}

// listedLine is the code of a signature line of a Go block, its comment cut
// off: gotest.Name, optional type parameters, the parameter list from t, an
// optional result.
var listedLine = regexp.MustCompile(`^gotest\.([A-Z]\w*)((?:\[[^\]]*\])?\(t(?:, .*)?\)(?: [\w.*\[\]]+)?)$`)

// listedSignatures reads the signature lines of the Go blocks of doc: a line
// whose every parameter after t names its type. A call such as
// gotest.Equal(t, want, got) is an example, not a signature, and is passed over.
func listedSignatures(doc string) map[string][]listed {
	out := map[string][]listed{}
	inGo := false
	for i, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "```") {
			inGo = strings.TrimSpace(line) == "```go"
			continue
		}
		code, _, _ := strings.Cut(line, "//")
		m := listedLine.FindStringSubmatch(strings.TrimSpace(code))
		if !inGo || m == nil {
			continue
		}
		src := "package p\nfunc " + m[1] + strings.Replace(strings.ReplaceAll(m[2], "gotest.", ""), "(t", "(t testingT", 1)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		fn := f.Decls[0].(*ast.FuncDecl)
		if slices.ContainsFunc(fn.Type.Params.List[1:], func(p *ast.Field) bool { return len(p.Names) == 0 }) {
			continue
		}
		out[m[1]] = append(out[m[1]], listed{line: i + 1, sig: signatureOf(fset, fn.Type)})
	}
	return out
}

type listed struct {
	line int
	sig  signature
}

const skillAPI = "skills/writing-gotest-tests/reference/api.md"

func (s *ReferenceTestSuite) TestTheSkillListsEveryAssertion(t *gotest.T) {
	source := assertions(t)
	lists := listedSignatures(readDoc(t, skillAPI))
	t.When("the skill's API reference is read for signature lines", func(w *gotest.T) {
		w.It("lists every assertion of pkg/gotest exactly once", func(it *gotest.T) {
			var wrong []string
			for _, name := range slices.Sorted(maps.Keys(source)) {
				if n := len(lists[name]); n != 1 {
					wrong = append(wrong, fmt.Sprintf("%s is listed %d times", name, n))
				}
			}
			gotest.Empty(it, wrong, "%s lists each assertion once, in its group", skillAPI)
		})
		w.It("lists each with the type parameters, parameters and result of its source", func(it *gotest.T) {
			var wrong []string
			for name, ls := range lists {
				want, ok := source[name]
				if !ok {
					continue // gotest.Go, gotest.Each: not assertions
				}
				for _, l := range ls {
					if l.sig != want {
						wrong = append(wrong, fmt.Sprintf("%s:%d: %s lists %s, pkg/gotest declares %s", skillAPI, l.line, name, l.sig, want))
					}
				}
			}
			slices.Sort(wrong)
			gotest.Empty(it, wrong)
		})
	})
}

func (s *ReferenceTestSuite) TestTheTourAndTheWebsiteNameEveryAssertion(t *gotest.T) {
	source := assertions(t)
	readme, site := readmeTour(t), readDoc(t, "site/content/reference.html")
	t.When("the README and the website reference are read", func(w *gotest.T) {
		w.It("the README's assertion tour calls every assertion", func(it *gotest.T) {
			var missing []string
			for _, name := range slices.Sorted(maps.Keys(source)) {
				if !regexp.MustCompile(`gotest\.` + name + `[(\[]`).MatchString(readme) {
					missing = append(missing, name)
				}
			}
			gotest.Empty(it, missing, "README.md's Type-Safe Assertions tour calls no gotest.<Name> for these; add each to its group")
		})
		w.It("the website reference tables every assertion", func(it *gotest.T) {
			var missing []string
			for _, name := range slices.Sorted(maps.Keys(source)) {
				if !regexp.MustCompile(`<code>` + name + `[(\[]`).MatchString(site) {
					missing = append(missing, name)
				}
			}
			gotest.Empty(it, missing, "site/content/reference.html has no row for these; add each to its group")
		})
	})
}

// readmeTour is the README's "Type-Safe Assertions" section, to the next heading.
func readmeTour(t *gotest.T) string {
	_, tour, found := strings.Cut(readDoc(t, "README.md"), "### Type-Safe Assertions\n")
	gotest.True(t, found, "README.md has no Type-Safe Assertions section")
	if end := strings.Index(tour, "\n#"); end >= 0 {
		tour = tour[:end]
	}
	return tour
}

func (s *ReferenceTestSuite) TestTheConfigFieldsAreNamed(t *gotest.T) {
	skill, site := readDoc(t, skillAPI), readDoc(t, "site/content/reference.html")
	for _, typeName := range []string{"SuiteConfig", "FixtureConfig"} {
		fields := configFields(t, typeName)
		t.When("the fields of "+typeName+" are looked up", func(w *gotest.T) {
			w.It("the website reference tables each", func(it *gotest.T) {
				for _, f := range fields {
					gotest.Contains(it, site, "<td><code>"+f+"</code></td>", "%s.%s has no row in site/content/reference.html", typeName, f)
				}
			})
			if typeName == "SuiteConfig" {
				w.It("the skill names each", func(it *gotest.T) {
					for _, f := range fields {
						gotest.Contains(it, skill, "`"+f+"`", "%s.%s is not named in %s", typeName, f, skillAPI)
					}
				})
			}
		})
	}
}

// userEnv are the environment variables a user may set; the design spec
// tables each. internalEnv are those gotest sets for its own processes.
var (
	userEnv     = []string{"GOTEST_CI", "GOTEST_AGENT", "GOTEST_CACHE_DIR", "GOTEST_SLOW_BUILD_THRESHOLD", "GOTEST_UPDATE_SNAPSHOTS", "GOTEST_TIMEOUT_SCALE"}
	internalEnv = []string{"GOTEST_SHARED_STATE_FILE", "GOTEST_SHARED_PROTOCOL_FD", "GOTEST_FUZZ_ECHO_INPUT", "GOTEST_CONTROL", "GOTEST_INTERNAL_INTERRUPT_CONSOLE", "GOTEST_STOP_FILE", "GOTEST_ASSERTIONS_FILE"}
	envName     = regexp.MustCompile(`^GOTEST_[A-Z0-9_]+$`)
	// testPlumbing are prefixes of the variables only gotest's own tests set.
	testPlumbing = []string{"GOTEST_TEST_", "GOTEST_MP_", "GOTEST_PROCTREE_", "GOTEST_CONTROL_TEST_"}
)

// envLiterals maps each GOTEST_ string literal of the non-test Go files under
// cmd, internal and pkg to the first place naming it. Test data, the testkit
// and the drill are test plumbing and not read.
func envLiterals(t *gotest.T) map[string]string {
	seen := map[string]string{}
	for _, dir := range []string{"cmd", "internal", "pkg"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(path)
			if d.IsDir() {
				if d.Name() == "testdata" || strings.HasSuffix(slash, "/internal/testkit") || strings.HasSuffix(slash, "/internal/drill") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				name, err := strconv.Unquote(lit.Value)
				if err != nil || !envName.MatchString(name) || slices.ContainsFunc(testPlumbing, func(p string) bool { return strings.HasPrefix(name, p) }) {
					return true
				}
				if _, ok := seen[name]; !ok {
					seen[name] = fmt.Sprintf("%s:%d", slash, fset.Position(lit.Pos()).Line)
				}
				return true
			})
			return nil
		})
		gotest.NoError(t, err)
	}
	return seen
}

func (s *ReferenceTestSuite) TestEveryEnvironmentVariableIsKnown(t *gotest.T) {
	seen := envLiterals(t)
	known := slices.Concat(userEnv, internalEnv)
	spec := readDoc(t, "docs/design/spec.md")
	// The table under "User-facing:", to the blank line after it.
	_, userTable, found := strings.Cut(spec, "User-facing:")
	gotest.True(t, found, "docs/design/spec.md has no user-facing environment table")
	header := strings.Index(userTable, "| Variable")
	gotest.GreaterOrEqual(t, header, 0, "docs/design/spec.md's user-facing table has no header")
	if end := strings.Index(userTable[header:], "\n\n"); end >= 0 {
		userTable = userTable[:header+end]
	}
	t.When("the code's GOTEST_ literals are compared with the known variables", func(w *gotest.T) {
		w.It("each one the code names is a user or an internal variable", func(it *gotest.T) {
			var unknown []string
			for _, name := range slices.Sorted(maps.Keys(seen)) {
				if !slices.Contains(known, name) {
					unknown = append(unknown, name+" at "+seen[name])
				}
			}
			gotest.Empty(it, unknown, "add each to userEnv, and to docs/design/spec.md's user table, or to internalEnv")
		})
		w.It("each known one is still named by the code", func(it *gotest.T) {
			var gone []string
			for _, name := range known {
				if _, ok := seen[name]; !ok {
					gone = append(gone, name)
				}
			}
			gotest.Empty(it, gone, "no code names these; remove them from the lists")
		})
	})
	t.When("the design spec's user-facing table is read", func(w *gotest.T) {
		w.It("tables exactly the user variables", func(it *gotest.T) {
			var tabled []string
			for _, m := range regexp.MustCompile("(?m)^\\| `(GOTEST_[A-Z0-9_]+)").FindAllStringSubmatch(userTable, -1) {
				tabled = append(tabled, m[1])
			}
			gotest.ElementsMatch(it, userEnv, tabled)
		})
	})
}
