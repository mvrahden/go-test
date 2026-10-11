package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/mvrahden/go-test/internal/about"
)

var gotestImport = about.Repo + "/pkg/gotest"

const (
	suitePkg   = "github.com/stretchr/testify/suite"
	assertPkg  = "github.com/stretchr/testify/assert"
	requirePkg = "github.com/stretchr/testify/require"
)

// assertionMap maps testify assertion method names to gotest function names.
var assertionMap = map[string]string{
	"Equal":          "Equal",
	"NotEqual":       "NotEqual",
	"NoError":        "NoError",
	"Error":          "Error",
	"ErrorIs":        "ErrorIs",
	"ErrorContains":  "ErrorContains",
	"True":           "True",
	"False":          "False",
	"Nil":            "Nil",
	"NotNil":         "NotNil",
	"Empty":          "Empty",
	"NotEmpty":       "NotEmpty",
	"Len":            "Len",
	"Contains":       "Contains",
	"Zero":           "Zero",
	"Greater":        "Greater",
	"GreaterOrEqual": "GreaterOrEqual",
	"Less":           "Less",
	"LessOrEqual":    "LessOrEqual",
}

// lifecycleRenames maps the testify hooks gotest has an equivalent for.
var lifecycleRenames = map[string]string{
	"SetupSuite":    "BeforeAll",
	"TearDownSuite": "AfterAll",
	"SetupTest":     "BeforeEach",
	"TearDownTest":  "AfterEach",
}

// unsupportedHooks are testify lifecycle hooks that have no gotest equivalent
// and cannot be converted automatically.
var unsupportedHooks = map[string]bool{
	"SetupSubTest":    true,
	"TearDownSubTest": true,
	"BeforeTest":      true,
	"AfterTest":       true,
	"HandleStats":     true,
}

// testifyAssertionNames is a superset of testify assertion method names. It is
// used to recognize unconverted direct s.<Method>(...) assertion calls on the
// suite receiver, where the method name alone must identify an assertion.
var testifyAssertionNames = map[string]bool{
	"Condition": true, "DirExists": true, "ElementsMatch": true,
	"EqualError": true, "EqualExportedValues": true, "EqualValues": true,
	"ErrorAs": true, "Eventually": true, "EventuallyWithT": true,
	"Exactly": true, "FileExists": true, "Implements": true,
	"InDelta": true, "InDeltaMapValues": true, "InDeltaSlice": true,
	"InEpsilon": true, "InEpsilonSlice": true, "IsDecreasing": true,
	"IsIncreasing": true, "IsNonDecreasing": true, "IsNonIncreasing": true,
	"IsType": true, "JSONEq": true, "Negative": true, "Never": true,
	"NoDirExists": true, "NoFileExists": true, "NotElementsMatch": true,
	"NotErrorAs": true, "NotErrorIs": true, "NotImplements": true,
	"NotPanics": true, "NotRegexp": true, "NotSame": true,
	"NotSubset": true, "NotZero": true, "Panics": true,
	"PanicsWithError": true, "PanicsWithValue": true, "Positive": true,
	"Regexp": true, "Same": true, "Subset": true,
	"WithinDuration": true, "WithinRange": true, "YAMLEq": true,
}

// isTestifyAssertionName reports whether name is a known testify assertion
// method (including mapped ones and formatted "...f" variants).
func isTestifyAssertionName(name string) bool {
	if testifyAssertionNames[name] || assertionMap[name] != "" {
		return true
	}
	if base, ok := strings.CutSuffix(name, "f"); ok {
		return testifyAssertionNames[base] || assertionMap[base] != ""
	}
	return false
}

// MigrateResult describes what was migrated in a file, or why it was not.
type MigrateResult struct {
	File    string
	OldName string
	NewName string
	// Markers counts the TODO(gotest-migrate) lines the file was left with,
	// including those an earlier run placed.
	Markers int
	// Refusal names why the file was not migrated at all; markers stand
	// where the reason was found.
	Refusal string
	// RefsOnly marks a file that declares no suite but was changed for
	// suites other files declare: their methods, runners or references.
	RefsOnly bool
}

// DeriveNewName computes the new suite name from an old one.
// If the name already ends with "TestSuite", it is left as-is.
// If it ends with "Suite", "Suite" is replaced with "TestSuite".
// Otherwise, "TestSuite" is appended.
func DeriveNewName(old string) string {
	if strings.HasSuffix(old, "TestSuite") {
		return old
	}
	if strings.HasSuffix(old, "Suite") {
		return old[:len(old)-len("Suite")] + "TestSuite"
	}
	return old + "TestSuite"
}

// Options steer a migration run.
type Options struct {
	// DryRun writes nothing; the unified diff of every edit goes to Out.
	DryRun bool
	Out    io.Writer
}

// markerPrefix opens every TODO(gotest-migrate) comment.
const markerPrefix = "// TODO(gotest-migrate):"

// countMarkers counts the TODO(gotest-migrate) comments in f; the text in a
// string literal is not one.
func countMarkers(f *ast.File) int {
	n := 0
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, markerPrefix) {
				n++
			}
		}
	}
	return n
}

// countSourceMarkers is countMarkers for formatted source.
func countSourceMarkers(src []byte) (int, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	return countMarkers(f), nil
}

// MigrateFile migrates a single file on its own, as if it were the only
// file of its package.
func MigrateFile(path string) ([]MigrateResult, error) {
	return migrateUnit("", []string{path}, nil, Options{})
}

// MigratePackages migrates the test files of the packages patterns name:
// directories (dir/... for every package below dir, as the go command reads
// it) or import paths. A directory is migrated as a whole: a suite's
// methods, runner and references may sit in any of its files, or in its
// external _test package.
func MigratePackages(patterns []string, opts Options) ([]MigrateResult, error) {
	var dirs []string
	seen := map[string]bool{}
	for _, pattern := range patterns {
		matched, err := patternDirs(pattern)
		if err != nil {
			return nil, err
		}
		for _, dir := range matched {
			// Overlapping patterns name a directory once.
			key, err := filepath.Abs(dir)
			if err != nil {
				return nil, err
			}
			if !seen[key] {
				seen[key] = true
				dirs = append(dirs, dir)
			}
		}
	}

	var all []MigrateResult
	for _, dir := range dirs {
		tests := testFiles(dir)
		if len(tests) == 0 {
			continue
		}
		results, err := migrateUnit(importPath(dir), tests, contextFiles(dir), opts)
		all = append(all, results...)
		if err != nil {
			return all, err
		}
	}
	return all, nil
}

// patternDirs lists the directories pattern names. A path (./x, ../x, /x)
// is read from disk, a path to a file as its directory; anything else is an
// import path pattern the go command resolves.
func patternDirs(pattern string) ([]string, error) {
	if !build.IsLocalImport(pattern) && !filepath.IsAbs(pattern) {
		return listDirs(pattern)
	}
	root, recursive := strings.CutSuffix(pattern, "/...")
	if root == "" {
		root = "."
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("pattern %s: %w", pattern, err)
	}
	if !info.IsDir() {
		return []string{filepath.Dir(root)}, nil
	}
	if !recursive {
		return []string{root}, nil
	}
	var dirs []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root {
			// What ./... leaves out: testdata, vendor, hidden and _ dirs,
			// and nested modules.
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		dirs = append(dirs, path)
		return nil
	})
	return dirs, err
}

// listDirs resolves an import path pattern with the go command, as the
// rest of the CLI does. A directory inside the working directory is named
// relative to it.
func listDirs(pattern string) ([]string, error) {
	cmd := exec.Command("go", "list", "-find", "-f", "{{.Dir}}", pattern)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("pattern %s: %s", pattern, msg)
	}
	wd, _ := os.Getwd()
	var dirs []string
	for _, dir := range strings.Fields(string(out)) {
		if rel, err := filepath.Rel(wd, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			dir = rel
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// testFiles lists the _test.go files of dir.
func testFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths
}

// contextFiles lists the non-test Go files of dir the default build would
// compile. They are not migrated; they only resolve names test code uses.
func contextFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, name); err == nil && ok {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	return paths
}

// importPath is the import path of dir, from the enclosing go.mod; empty
// outside a module. An external _test package reaches the package's own
// suites through it.
func importPath(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for root := abs; ; {
		data, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil {
			mod := modfile.ModulePath(data)
			if mod == "" {
				return ""
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return ""
			}
			if rel == "." {
				return mod
			}
			return mod + "/" + filepath.ToSlash(rel)
		}
		parent := filepath.Dir(root)
		if parent == root {
			return ""
		}
		root = parent
	}
}

// migrateUnit migrates the test files tests together, writes what changed
// (or prints its diff) and reports per file.
func migrateUnit(importPath string, tests, context []string, opts Options) ([]MigrateResult, error) {
	sort.Strings(tests)
	u, err := loadUnit(importPath, tests, context)
	if err != nil {
		return nil, err
	}
	if err := u.migrate(); err != nil {
		return nil, err
	}

	var results []MigrateResult
	for _, uf := range u.files {
		if !uf.test {
			continue
		}
		final := uf.src
		changed := uf.out != nil && !bytes.Equal(uf.out, uf.src)
		if changed {
			final = uf.out
		}
		markers, err := countSourceMarkers(final)
		if err != nil {
			return results, fmt.Errorf("annotate %s: %w", uf.path, err)
		}
		results = append(results, u.fileResults(uf, changed, markers)...)
		if !changed {
			continue
		}
		if opts.DryRun {
			if opts.Out != nil {
				fmt.Fprint(opts.Out, unifiedDiff(uf.path, uf.src, uf.out))
			}
			continue
		}
		if err := os.WriteFile(uf.path, uf.out, 0644); err != nil { //nolint:gosec // G306: not sensitive data
			return results, fmt.Errorf("write %s: %w", uf.path, err)
		}
	}
	return results, nil
}

// fileResults reports one file: a result per suite it declares, its
// refusal, or the markers left in a file changed only for other files'
// suites or not changed at all.
func (u *unit) fileResults(uf *unitFile, changed bool, markers int) []MigrateResult {
	if uf.refused {
		reasons := make([]string, 0, len(uf.marks))
		for _, m := range uf.marks {
			reasons = append(reasons, m.msg)
		}
		return []MigrateResult{{File: uf.path, Markers: markers, Refusal: strings.Join(reasons, "; ")}}
	}
	var results []MigrateResult
	for _, s := range u.suites {
		if s.file != uf || !s.migrated() {
			continue
		}
		results = append(results, MigrateResult{
			File:    uf.path,
			OldName: s.obj.Name(),
			NewName: s.newName,
			Markers: markers,
		})
		markers = 0 // the file's markers are counted once, on its first suite
	}
	switch {
	case len(results) > 0:
		return results
	case changed:
		return []MigrateResult{{File: uf.path, Markers: markers, RefsOnly: true}}
	case markers > 0:
		return []MigrateResult{{File: uf.path, Markers: markers}}
	}
	return nil
}
