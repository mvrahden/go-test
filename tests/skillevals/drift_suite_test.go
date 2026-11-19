package skillevals_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SkillDriftTestSuite: what the skill names exists in the code it describes.
type SkillDriftTestSuite struct{}

func (s *SkillDriftTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// placeholderSymbols are names the skill uses as stand-ins, not as API.
var placeholderSymbols = map[string]bool{"X": true}

// placeholderFlags are flags the skill uses as stand-ins.
var placeholderFlags = map[string]bool{"--x": true, "--double-dash": true}

// foreignFlags are other tools' flags the skill shows in their commands:
// gh, docker and git apply.
var foreignFlags = map[string]bool{
	"--state": true, "--search": true, "--title": true, "--body-file": true, // gh
	"--tmpfs": true, "--init": true, // docker
	"--unsafe-paths": true, // git apply
}

var (
	symbolToken = regexp.MustCompile(`\bgotest\.([A-Z][A-Za-z0-9]*)`)
	flagToken   = regexp.MustCompile(`(?:^|[^-\w])(--[a-z][a-z0-9-]*)`)
	flagRow     = regexp.MustCompile("(?m)^\\| `(--[a-z][a-z0-9-]*)")
)

// repoRoot is the repository the suite runs in.
func repoRoot(t *gotest.T) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	return root
}

// skillFile is one Markdown file of the skill, by its path from the skill's directory.
type skillFile struct {
	rel   string
	lines []string
}

// skillFiles reads SKILL.md and every reference/*.md, sorted by path.
func skillFiles(t *gotest.T) []skillFile {
	dir := filepath.Join(repoRoot(t), "skills", "writing-gotest-tests")
	paths := []string{"SKILL.md"}
	entries, err := os.ReadDir(filepath.Join(dir, "reference"))
	gotest.NoError(t, err)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			paths = append(paths, "reference/"+e.Name())
		}
	}
	sort.Strings(paths)
	files := make([]skillFile, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p))) //nolint:gosec // G304: the repository's own skill
		gotest.NoError(t, err)
		files = append(files, skillFile{rel: p, lines: strings.Split(string(data), "\n")})
	}
	return files
}

// registeredFlags reads the flags the CLI registers from the Flags table of
// docs/design/spec.md, which the surface suite holds to the flag registry.
func registeredFlags(t *gotest.T) map[string]bool {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "design", "spec.md"))
	gotest.NoError(t, err)
	_, section, found := strings.Cut(string(data), "\n### Flags\n")
	gotest.True(t, found, "docs/design/spec.md has no Flags section")
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}
	flags := map[string]bool{}
	for _, m := range flagRow.FindAllStringSubmatch(section, -1) {
		flags[m[1]] = true
	}
	gotest.NotEmpty(t, flags, "the Flags section lists no flag")
	return flags
}

// exportedNames collects the exported top-level names of pkg/gotest.
func exportedNames(t *gotest.T) map[string]bool {
	fset := token.NewFileSet()
	notTest := func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot(t), "pkg", "gotest"), notTest, parser.SkipObjectResolution) //nolint:staticcheck // ParseDir suffices for top-level names
	gotest.NoError(t, err)
	names := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil && d.Name.IsExported() {
						names[d.Name.Name] = true
					}
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						switch sp := spec.(type) {
						case *ast.TypeSpec:
							if sp.Name.IsExported() {
								names[sp.Name.Name] = true
							}
						case *ast.ValueSpec:
							for _, n := range sp.Names {
								if n.IsExported() {
									names[n.Name] = true
								}
							}
						}
					}
				}
			}
		}
	}
	gotest.NotEmpty(t, names, "pkg/gotest exports nothing")
	return names
}

func (s *SkillDriftTestSuite) TestSymbolsTheSkillNamesExist(t *gotest.T) {
	exported := exportedNames(t)
	var missing []string
	for _, f := range skillFiles(t) {
		for i, line := range f.lines {
			for _, m := range symbolToken.FindAllStringSubmatch(line, -1) {
				if !exported[m[1]] && !placeholderSymbols[m[1]] {
					missing = append(missing, fmt.Sprintf("%s:%d: gotest.%s", f.rel, i+1, m[1]))
				}
			}
		}
	}
	t.When("the skill names a gotest symbol", func(w *gotest.T) {
		w.It("is exported by pkg/gotest", func(it *gotest.T) {
			gotest.Empty(it, missing, "pkg/gotest exports no such name; fix the skill or the name:\n%s", strings.Join(missing, "\n"))
		})
	})
}

func (s *SkillDriftTestSuite) TestFlagsTheSkillNamesExist(t *gotest.T) {
	registered := registeredFlags(t)
	var unknown []string
	for _, f := range skillFiles(t) {
		for i, line := range f.lines {
			for _, m := range flagToken.FindAllStringSubmatch(line, -1) {
				flag := m[1]
				if !registered[flag] && !placeholderFlags[flag] && !foreignFlags[flag] {
					unknown = append(unknown, fmt.Sprintf("%s:%d: %s", f.rel, i+1, flag))
				}
			}
		}
	}
	t.When("the skill names a --flag", func(w *gotest.T) {
		w.It("is a flag of the gotest CLI", func(it *gotest.T) {
			gotest.Empty(it, unknown, "gotest has no such flag; fix the skill, or list another tool's flag in foreignFlags:\n%s", strings.Join(unknown, "\n"))
		})
	})
}

func (s *SkillDriftTestSuite) TestTheCLIReferenceNamesEveryFlag(t *gotest.T) {
	named := map[string]bool{}
	for _, f := range skillFiles(t) {
		if f.rel != "reference/cli.md" {
			continue
		}
		for _, line := range f.lines {
			for _, m := range flagToken.FindAllStringSubmatch(line, -1) {
				named[m[1]] = true
			}
		}
	}
	var absent []string
	for flag := range registeredFlags(t) {
		if !named[flag] {
			absent = append(absent, flag)
		}
	}
	sort.Strings(absent)
	t.When("reference/cli.md is read as the full CLI surface", func(w *gotest.T) {
		w.It("names every flag the CLI registers", func(it *gotest.T) {
			gotest.Empty(it, absent, "reference/cli.md does not name: %s", strings.Join(absent, " "))
		})
	})
}
