package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/filewrite"
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
	"NotZero":        "NotZero",
	"Same":           "Same",
	"NotSame":        "NotSame",
	"ElementsMatch":  "ElementsMatch",
	"Subset":         "Subset",
	"NotContains":    "NotContains",
	"Regexp":         "Regexp",
	"InDelta":        "InDelta",
	"JSONEq":         "JSONEq",
	"Panics":         "Panics",
}

// gotestAssertion is the gotest function for a testify assertion, its
// formatted ...f variant included: gotest formats a trailing message and
// its args as testify did.
func gotestAssertion(name string) (gotestName, base string, ok bool) {
	if g, ok := assertionMap[name]; ok {
		return g, name, true
	}
	base = strings.TrimSuffix(name, "f")
	g, ok := assertionMap[base]
	return g, base, ok && base != name
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
	// Uncompiled lists, for a directory (File), the errors its packages
	// had before the migration; nothing in it was changed.
	Uncompiled []string
	// Unplaced lists, for a directory (File), the errors the migrated
	// packages have where no marker can stand.
	Unplaced []string
	// Generated marks a file headed "Code generated ... DO NOT EDIT." that
	// uses testify: migrate leaves it to its generator.
	Generated bool
}

// DeriveNewName computes the new suite name from an old one, exported:
// gotest runs only exported suites.
// If the name already ends with "TestSuite", it is left as-is.
// If it ends with "Suite", "Suite" is replaced with "TestSuite".
// Otherwise, "TestSuite" is appended.
func DeriveNewName(old string) string {
	r, size := utf8.DecodeRuneInString(old)
	name := string(unicode.ToUpper(r)) + old[size:]
	if strings.HasSuffix(name, "TestSuite") {
		return name
	}
	if strings.HasSuffix(name, "Suite") {
		return name[:len(name)-len("Suite")] + "TestSuite"
	}
	return name + "TestSuite"
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

// MigratePackages migrates the test files of the directories patterns name
// (see patternDirs). A directory is migrated as a whole: a suite's
// methods, runner and references may sit in any of its files, or in its
// external _test package. Its test packages are type-checked for real
// before and after, in every build configuration its test files need: one
// that does not compile before is left alone and reported in Uncompiled;
// every error the migrated code has gets a marker on its line, so a result
// without markers compiles.
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
			key, err := canonical(dir)
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
	var jobs []*dirJob
	shown := map[string]string{} // canonical path -> the path to show
	for _, dir := range dirs {
		abs, err := canonical(dir)
		if err != nil {
			return nil, err
		}
		tests := testFiles(abs)
		if len(tests) == 0 {
			continue
		}
		for _, path := range tests {
			shown[path] = filepath.Join(dir, filepath.Base(path))
		}
		if !usesTestifySuite(tests) {
			// Nothing to convert; only an earlier run's markers count.
			results, err := leftoverResults(tests, shown)
			if err != nil {
				return all, err
			}
			all = append(all, results...)
			continue
		}
		modRoot := moduleRoot(abs)
		if modRoot == "" {
			return all, fmt.Errorf("%s: not inside a Go module; migrate type-checks the packages it converts", dir)
		}
		jobs = append(jobs, newDirJob(abs, modRoot, tests))
	}
	if len(jobs) == 0 {
		return all, nil
	}

	fset := token.NewFileSet()
	srcs := map[string][]byte{}
	before, err := loadJobs(jobs, nil, fset, srcs)
	if err != nil {
		return all, err
	}
	var units []*jobUnit
	for _, j := range jobs {
		if errs := errorsOf(before[j]); len(errs) > 0 {
			// An earlier run's markers explain a package it left not
			// compiling; they are what is left to do.
			results, err := leftoverResults(j.tests, shown)
			if err != nil {
				return all, err
			}
			if len(results) == 0 {
				results = []MigrateResult{{File: shownDir(j, shown), Uncompiled: errorStrings(errs, shown)}}
			}
			all = append(all, results...)
			continue
		}
		u, err := newUnit(j, before[j], fset, srcs)
		if err != nil {
			return all, err
		}
		if err := u.migrate(); err != nil {
			return all, err
		}
		units = append(units, &jobUnit{job: j, u: u})
	}

	if err := verify(units, shown); err != nil {
		return all, err
	}

	var changes []filewrite.Change
	for _, ju := range units {
		results, changed, err := ju.write(opts, shown)
		all = append(all, results...)
		if err != nil {
			return all, err
		}
		changes = append(changes, changed...)
	}
	return all, writeAll(changes, shown)
}

// writeAll writes every changed file or none (see filewrite.Stage); the
// error says which were written.
func writeAll(changes []filewrite.Change, shown map[string]string) error {
	if len(changes) == 0 {
		return nil
	}
	staged, err := filewrite.Stage(changes)
	if err != nil {
		return fmt.Errorf("writing the migrated files: %w; nothing was written", err)
	}
	n, err := staged.Commit()
	if err == nil {
		return nil
	}
	if n == 0 {
		return fmt.Errorf("writing the migrated files: %w; nothing was written", err)
	}
	names := func(cs []filewrite.Change) string {
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, shown[c.File])
		}
		return strings.Join(out, ", ")
	}
	return fmt.Errorf("writing the migrated files: %w; wrote %s; did not write %s", err, names(changes[:n]), names(changes[n:]))
}

// jobUnit is a directory's migration and what its type-check found.
type jobUnit struct {
	job      *dirJob
	u        *unit
	unplaced []string
}

// verify type-checks every migrated directory from its migrated sources
// and marks what still needs a hand: the constructs the migration knows it
// could not convert, and every error the result has, on its line.
func verify(units []*jobUnit, shown map[string]string) error {
	overlay := map[string][]byte{}
	var jobs []*dirJob
	byJob := map[*dirJob]*jobUnit{}
	for _, ju := range units {
		changed := false
		for _, uf := range ju.u.files {
			if uf.out != nil && !bytes.Equal(uf.out, uf.src) {
				overlay[uf.path] = uf.out
				changed = true
			}
		}
		if changed {
			jobs = append(jobs, ju.job)
			byJob[ju.job] = ju
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	fset := token.NewFileSet()
	srcs := map[string][]byte{}
	after, err := loadJobs(jobs, overlay, fset, srcs)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		ju := byJob[j]
		au, err := newUnit(j, after[j], fset, srcs)
		if err != nil {
			return err
		}
		au.byKey = map[string]*suiteDecl{}
		for _, s := range ju.u.suites {
			if s.migrated() {
				au.byKey[s.obj.Pkg().Path()+"."+s.newName] = s
			}
		}
		marks := map[string]*lineMarks{}
		for i, uf := range ju.u.files {
			if uf.generated {
				continue // its errors have no marker to carry them
			}
			m := &lineMarks{fset: fset}
			marks[uf.path] = m
			if !uf.refused && uf.unchecked == "" {
				au.unconverted(au.files[i].f, m)
			}
		}
		for _, e := range errorsOf(after[j]) {
			if m := marks[e.file]; m != nil && e.line > 0 {
				m.addLine(e.line, "does not compile: "+strings.Join(strings.Fields(e.msg), " "))
				continue
			}
			ju.unplaced = append(ju.unplaced, errorString(e, shown))
		}
		for _, uf := range ju.u.files {
			m := marks[uf.path]
			if m == nil || len(m.anns) == 0 {
				continue
			}
			current := uf.src
			if uf.out != nil {
				current = uf.out
			}
			out, err := insertMarkers(current, m.anns)
			if err != nil {
				return fmt.Errorf("annotate %s: %w", uf.path, err)
			}
			uf.out = out
		}
	}
	return nil
}

// write returns what changed in the directory, for writeAll (or prints its
// diff), and reports per file.
func (ju *jobUnit) write(opts Options, shown map[string]string) ([]MigrateResult, []filewrite.Change, error) {
	var results []MigrateResult
	var changes []filewrite.Change
	for _, uf := range ju.u.files {
		final := uf.src
		changed := uf.out != nil && !bytes.Equal(uf.out, uf.src)
		if changed {
			final = uf.out
		}
		markers, err := countSourceMarkers(final)
		if err != nil {
			return results, nil, fmt.Errorf("annotate %s: %w", uf.path, err)
		}
		results = append(results, ju.u.fileResults(uf, shown[uf.path], changed, markers)...)
		if !changed {
			continue
		}
		if opts.DryRun {
			if opts.Out != nil {
				fmt.Fprint(opts.Out, unifiedDiff(shown[uf.path], uf.src, uf.out))
			}
			continue
		}
		changes = append(changes, filewrite.Change{File: uf.path, Before: uf.src, After: uf.out})
	}
	if len(ju.unplaced) > 0 {
		results = append(results, MigrateResult{File: shownDir(ju.job, shown), Unplaced: ju.unplaced})
	}
	return results, changes, nil
}

// fileResults reports one file: a result per suite it declares, its
// refusal, or the markers left in a file changed only for other files'
// suites or not changed at all.
func (u *unit) fileResults(uf *unitFile, shown string, changed bool, markers int) []MigrateResult {
	if uf.generated {
		if importsSuite(uf.f) {
			return []MigrateResult{{File: shown, Markers: markers, Generated: true}}
		}
		return nil
	}
	if uf.refused {
		reasons := make([]string, 0, len(uf.marks))
		for _, m := range uf.marks {
			reasons = append(reasons, m.msg)
		}
		return []MigrateResult{{File: shown, Markers: markers, Refusal: strings.Join(reasons, "; ")}}
	}
	var results []MigrateResult
	for _, s := range u.suites {
		if s.file != uf || !s.migrated() {
			continue
		}
		results = append(results, MigrateResult{
			File:    shown,
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
		return []MigrateResult{{File: shown, Markers: markers, RefsOnly: true}}
	case markers > 0:
		return []MigrateResult{{File: shown, Markers: markers}}
	}
	return nil
}

// leftoverResults reports the markers an earlier run left in files with
// nothing left to convert.
func leftoverResults(tests []string, shown map[string]string) ([]MigrateResult, error) {
	var results []MigrateResult
	for _, path := range tests {
		src, err := os.ReadFile(path)
		if err != nil {
			return results, fmt.Errorf("read %s: %w", path, err)
		}
		n, err := countSourceMarkers(src)
		if err != nil {
			return results, fmt.Errorf("parse %s: %w", path, err)
		}
		if n > 0 {
			results = append(results, MigrateResult{File: shown[path], Markers: n})
		}
	}
	return results, nil
}

// usesTestifySuite reports whether any of the files imports testify's
// suite package: without it there is no suite to convert.
func usesTestifySuite(paths []string) bool {
	for _, path := range paths {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return true // let the type-check report it
		}
		for _, is := range f.Imports {
			if strings.Trim(is.Path.Value, `"`) == suitePkg {
				return true
			}
		}
	}
	return false
}

func shownDir(j *dirJob, shown map[string]string) string {
	if len(j.tests) > 0 {
		if s, ok := shown[j.tests[0]]; ok {
			return filepath.Dir(s)
		}
	}
	return j.dir
}

func errorStrings(errs []loadErr, shown map[string]string) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, errorString(e, shown))
	}
	return out
}

// errorString is e with its file as the user named it.
func errorString(e loadErr, shown map[string]string) string {
	if s, ok := shown[e.file]; ok {
		e.file = s
	}
	return e.String()
}

// canonical is dir absolute with symlinks resolved, as the go command
// reports the files it loads.
func canonical(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

// moduleRoot is the directory of the go.mod enclosing dir, empty outside
// a module.
func moduleRoot(dir string) string {
	for root := dir; ; {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return ""
		}
		root = parent
	}
}

// testFiles lists the _test.go files of dir, sorted.
func testFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		// The go command ignores files named _x or .x.
		if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, "_test.go") && name[0] != '_' && name[0] != '.' {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}
