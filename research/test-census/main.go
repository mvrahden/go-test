// Command test-census scans checked-out Go repositories and reports how their
// tests are written: frameworks, parallelism, sleeps, table-driven shape,
// golden-file conventions, container usage, and modern testing-API adoption.
//
// Usage: test-census -out results.csv <repo-dir> [<repo-dir>...]
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	generatedRe    = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.?$`)
	composeNameRe  = regexp.MustCompile(`^(docker-)?compose[^/]*\.ya?ml$`)
	composeImageRe = regexp.MustCompile(`^\s*image:\s*["']?([^"'\s#]+)`)
	// plausible image reference: registry/repo path, optionally tagged; no
	// interpolation vars, at least 3 chars in the name part
	imageRefRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{2,}(:[A-Za-z0-9._-]+)?$`)
)

func plausibleImage(img string) bool {
	return imageRefRe.MatchString(img)
}

var frameworkImports = map[string]string{
	"github.com/stretchr/testify/assert":          "testify",
	"github.com/stretchr/testify/require":         "testify",
	"github.com/stretchr/testify/mock":            "testify-mock",
	"github.com/stretchr/testify/suite":           "testify-suite",
	"github.com/onsi/ginkgo":                      "ginkgo",
	"github.com/onsi/ginkgo/v2":                   "ginkgo",
	"github.com/onsi/gomega":                      "gomega",
	"gopkg.in/check.v1":                           "gocheck",
	"github.com/smartystreets/goconvey/convey":    "goconvey",
	"go.uber.org/mock/gomock":                     "gomock",
	"github.com/golang/mock/gomock":               "gomock",
	"github.com/testcontainers/testcontainers-go": "testcontainers",
	"github.com/ory/dockertest":                   "dockertest",
	"github.com/ory/dockertest/v3":                "dockertest",
	"github.com/bradleyjkemp/cupaloy":             "snapshot-lib",
	"github.com/bradleyjkemp/cupaloy/v2":          "snapshot-lib",
	"github.com/gkampitakis/go-snaps/snaps":       "snapshot-lib",
	"github.com/sebdah/goldie":                    "snapshot-lib",
	"github.com/sebdah/goldie/v2":                 "snapshot-lib",
	"github.com/hexops/autogold":                  "snapshot-lib",
	"github.com/hexops/autogold/v2":               "snapshot-lib",
	"github.com/google/go-cmp/cmp":                "go-cmp",
	"net/http/httptest":                           "httptest",
	"testing/quick":                               "quick",
}

var skipDirs = map[string]bool{
	"vendor": true, "third_party": true,
	"node_modules": true, ".git": true, "Godeps": true,
}

func isInfraImport(p string) bool {
	return strings.HasPrefix(p, "github.com/testcontainers/testcontainers-go") ||
		strings.HasPrefix(p, "github.com/ory/dockertest")
}

type repoStats struct {
	Name string
	SHA  string

	TestFiles     int
	TestLOC       int
	NonTestLOC    int
	PkgsWithTests int

	TestFuncs      int
	FuzzFuncs      int
	BenchFuncs     int
	TestMainPkgs   int
	GinkgoSpecs    int
	SubtestCalls   int
	SubtestMaxD    int
	ParallelTests  int // Test funcs calling t.Parallel() at top level
	ParallelSubs   int // Parallel() calls inside subtest closures
	TableTests     int
	UnderscoreName int

	SleepCalls   int
	SleepKnownMS int64

	CleanupCalls int
	HelperCalls  int
	SkipCalls    int
	ShortGuards  int

	Frameworks map[string]int // framework -> test files importing it

	GoldenFiles     int
	TestdataRefs    int // string literals referencing testdata/ in test files
	UpdateFlag      bool
	GoldenPractice  bool // update flag + testdata refs
	ContainerPkgs   int  // packages that reach container libs (direct or via local imports)
	TestMainTCPkgs  int  // packages with TestMain that reach container libs
	TCModules       map[string]int
	ContainerImages map[string]int
	ComposeImages   map[string]int

	BuildTagIntTests int
}

func newRepoStats(name string) *repoStats {
	return &repoStats{Name: name, Frameworks: map[string]int{},
		TCModules: map[string]int{}, ContainerImages: map[string]int{}, ComposeImages: map[string]int{}}
}

// pkgInfo accumulates per-directory facts during the walk.
type pkgInfo struct {
	localImports []string // repo-local package dirs imported by any file in this dir
	providesTC   bool     // any file imports testcontainers/dockertest
	hasTests     bool
	hasTestMain  bool
}

func main() {
	out := flag.String("out", "census.csv", "CSV output path")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: test-census -out results.csv <repo-dir>...")
		os.Exit(2)
	}
	var all []*repoStats
	for _, dir := range flag.Args() {
		st, err := scanRepo(filepath.Clean(dir))
		if err != nil {
			fmt.Fprintf(os.Stderr, "SKIP %s: %v\n", dir, err)
			continue
		}
		all = append(all, st)
		fmt.Printf("scanned %-28s files=%-4d tests=%-5d ginkgo=%-4d\n", st.Name, st.TestFiles, st.TestFuncs, st.GinkgoSpecs)
	}
	if err := writeCSV(*out, all); err != nil {
		fmt.Fprintln(os.Stderr, "csv:", err)
		os.Exit(1)
	}
	summarize(all)
}

func scanRepo(root string) (*repoStats, error) {
	base := filepath.Base(root)
	// clone dirs are owner_repo; GitHub owner names cannot contain "_", so the
	// first underscore is the separator
	if i := strings.Index(base, "_"); i > 0 {
		base = base[:i] + "/" + base[i+1:]
	}
	st := newRepoStats(base)
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		st.SHA = strings.TrimSpace(string(out))
	}
	modulePath := readModulePath(filepath.Join(root, "go.mod"))

	pkgs := map[string]*pkgInfo{}
	getPkg := func(dir string) *pkgInfo {
		p, ok := pkgs[dir]
		if !ok {
			p = &pkgInfo{}
			pkgs[dir] = p
		}
		return p
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] || strings.HasPrefix(name, "_") || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		base := d.Name()
		if strings.HasSuffix(base, ".golden") {
			st.GoldenFiles++
			return nil
		}
		if composeNameRe.MatchString(base) {
			// compose files inside fixture dirs are parser test-inputs, not
			// the repo's own test infrastructure
			if !strings.Contains(path, "fixtures") && !strings.Contains(path, "testdata") {
				scanComposeFile(path, st)
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") {
			return nil
		}
		inTestdata := strings.Contains(path, string(filepath.Separator)+"testdata"+string(filepath.Separator))
		if inTestdata {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil || isGenerated(src) {
			return nil
		}
		loc := strings.Count(string(src), "\n") + 1
		dir := filepath.Dir(path)
		pi := getPkg(dir)

		if !strings.HasSuffix(base, "_test.go") {
			st.NonTestLOC += loc
			f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
			if err != nil {
				return nil
			}
			infra := recordImports(f, pi, modulePath, root, nil, st)
			if infra {
				// re-parse fully to extract image strings from helper code
				if full, err := parser.ParseFile(fset, path, src, 0); err == nil {
					extractContainerDetail(full, st)
				}
			}
			return nil
		}

		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil
		}
		st.TestFiles++
		st.TestLOC += loc
		pi.hasTests = true
		if hasIntegrationTag(f) {
			st.BuildTagIntTests++
		}
		ginkgo := false
		recordImports(f, pi, modulePath, root, func(imp, fw string) {
			st.Frameworks[fw]++
			if fw == "ginkgo" {
				ginkgo = true
			}
		}, st)
		extractContainerDetail(f, st)
		analyzeTestFile(f, st, pi, ginkgo)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// container reachability: BFS from packages that import container libs
	reach := map[string]bool{}
	for dir, pi := range pkgs {
		if pi.providesTC {
			reach[dir] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for dir, pi := range pkgs {
			if reach[dir] {
				continue
			}
			for _, dep := range pi.localImports {
				if reach[dep] {
					reach[dir] = true
					changed = true
					break
				}
			}
		}
	}
	for dir, pi := range pkgs {
		if pi.hasTests {
			st.PkgsWithTests++
		}
		if pi.hasTestMain {
			st.TestMainPkgs++
		}
		if reach[dir] && pi.hasTests {
			st.ContainerPkgs++
			if pi.hasTestMain {
				st.TestMainTCPkgs++
			}
		}
	}
	st.GoldenPractice = st.UpdateFlag && st.TestdataRefs > 0
	return st, nil
}

// recordImports notes framework imports (via onFW, test files only), infra
// provision, and repo-local imports for the reachability graph. Reports
// whether the file imports a container library.
func recordImports(f *ast.File, pi *pkgInfo, modulePath, root string, onFW func(imp, fw string), st *repoStats) bool {
	infra := false
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if fw, ok := frameworkImports[p]; ok && onFW != nil {
			onFW(p, fw)
		}
		if isInfraImport(p) {
			pi.providesTC = true
			infra = true
			if rest, ok := strings.CutPrefix(p, "github.com/testcontainers/testcontainers-go/modules/"); ok {
				st.TCModules[strings.SplitN(rest, "/", 2)[0]]++
			}
		}
		if modulePath != "" && strings.HasPrefix(p, modulePath) {
			rel := strings.TrimPrefix(strings.TrimPrefix(p, modulePath), "/")
			pi.localImports = append(pi.localImports, filepath.Join(root, filepath.FromSlash(rel)))
		}
	}
	return infra
}

// extractContainerDetail pulls image references from testcontainers
// ContainerRequest{Image: "..."} literals and dockertest Run calls.
func extractContainerDetail(f *ast.File, st *repoStats) {
	hasDockertest := false
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); strings.HasPrefix(p, "github.com/ory/dockertest") {
			hasDockertest = true
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			name := typeName(n.Type)
			if name != "ContainerRequest" && name != "RunOptions" {
				return true
			}
			for _, el := range n.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := kv.Key.(*ast.Ident)
				if key == nil || (key.Name != "Image" && key.Name != "Repository") {
					continue
				}
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if img, _ := strconv.Unquote(lit.Value); plausibleImage(img) {
						st.ContainerImages[img]++
					}
				}
			}
		case *ast.CallExpr:
			// dockertest: pool.Run("postgres", "13", env) — image is arg 0
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && hasDockertest && sel.Sel.Name == "Run" && len(n.Args) == 3 {
				if lit, ok := n.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if img, _ := strconv.Unquote(lit.Value); plausibleImage(img) {
						st.ContainerImages[img]++
					}
				}
			}
		}
		return true
	})
}

func typeName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

// analyzeTestFile handles all per-file metrics with correct scoping:
// file-wide counters (sleep/cleanup/helper/skip) cover helpers too, while
// per-test analysis (parallel/table/subtests) walks each Test func.
func analyzeTestFile(f *ast.File, st *repoStats, pi *pkgInfo, ginkgo bool) {
	tvars := collectTParams(f)
	durs := collectDurConsts(f)

	// file-wide counters (helpers included)
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			if ginkgo {
				if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "It" || id.Name == "Specify" || id.Name == "Entry") {
					st.GinkgoSpecs++
				}
			}
			return true
		}
		x, _ := sel.X.(*ast.Ident)
		switch sel.Sel.Name {
		case "Sleep":
			if x != nil && x.Name == "time" {
				st.SleepCalls++
				if ms, ok := sleepMS(call, durs); ok {
					st.SleepKnownMS += ms
				}
			}
		case "Cleanup":
			if x != nil && tvars[x.Name] {
				st.CleanupCalls++
			}
		case "Helper":
			if x != nil && tvars[x.Name] {
				st.HelperCalls++
			}
		case "Skip", "Skipf", "SkipNow":
			if x != nil && tvars[x.Name] {
				st.SkipCalls++
			}
		case "Short":
			if x != nil && x.Name == "testing" {
				st.ShortGuards++
			}
		case "It", "Specify", "Entry":
			if ginkgo {
				st.GinkgoSpecs++
			}
		}
		// flag.Bool(...) with an update/golden flag name
		if x != nil && x.Name == "flag" && sel.Sel.Name == "Bool" && len(call.Args) > 0 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				v := strings.ToLower(lit.Value)
				if strings.Contains(v, "update") || strings.Contains(v, "golden") {
					st.UpdateFlag = true
				}
			}
		}
		return true
	})

	// testdata references anywhere in the file
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "testdata/") {
			st.TestdataRefs++
			return true
		}
		return true
	})

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			continue
		}
		name := fd.Name.Name
		switch {
		case name == "TestMain":
			pi.hasTestMain = true
		case strings.HasPrefix(name, "Test") && isTestSig(fd, "T"):
			st.TestFuncs++
			if strings.Contains(name[4:], "_") {
				st.UnderscoreName++
			}
			analyzeTestFunc(fd, st, tvars)
		case strings.HasPrefix(name, "Fuzz") && isTestSig(fd, "F"):
			st.FuzzFuncs++
		case strings.HasPrefix(name, "Benchmark") && isTestSig(fd, "B"):
			st.BenchFuncs++
		}
	}
}

// analyzeTestFunc walks one Test func tracking subtest depth so parallelism
// can be attributed to the test itself (depth 0) vs subtest closures.
func analyzeTestFunc(fd *ast.FuncDecl, st *repoStats, tvars map[string]bool) {
	table := false
	tableIdents := collectTableIdents(fd)
	parallelTop := false

	var walk func(n ast.Node, depth int)
	walk = func(node ast.Node, depth int) {
		ast.Inspect(node, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				// only descend via explicit t.Run handling below
				return false
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				x, _ := sel.X.(*ast.Ident)
				switch sel.Sel.Name {
				case "Parallel":
					if x != nil && tvars[x.Name] {
						if depth == 0 {
							parallelTop = true
						} else {
							st.ParallelSubs++
						}
					}
				case "Run":
					if x != nil && tvars[x.Name] {
						st.SubtestCalls++
						if depth+1 > st.SubtestMaxD {
							st.SubtestMaxD = depth + 1
						}
						for _, arg := range n.Args {
							if fl, ok := arg.(*ast.FuncLit); ok {
								walk(fl.Body, depth+1)
							}
						}
						return false // handled args ourselves
					}
				}
			case *ast.RangeStmt:
				if isTableRange(n, tableIdents) && bodyUsesT(n.Body, tvars) {
					table = true
				}
			}
			return true
		})
	}
	walk(fd.Body, 0)
	if parallelTop {
		st.ParallelTests++
	}
	if table {
		st.TableTests++
	}
}

// collectTableIdents finds idents assigned slice/map composite literals inside
// the func, so `for _, tc := range cases` is recognized as table-driven.
func collectTableIdents(fd *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(fd, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			cl, ok := rhs.(*ast.CompositeLit)
			if !ok || !isSliceOrMapType(cl.Type) {
				continue
			}
			if id, ok := assign.Lhs[i].(*ast.Ident); ok {
				out[id.Name] = true
			}
		}
		return true
	})
	return out
}

func isSliceOrMapType(e ast.Expr) bool {
	switch e.(type) {
	case *ast.ArrayType, *ast.MapType:
		return true
	}
	return false
}

func isTableRange(r *ast.RangeStmt, tableIdents map[string]bool) bool {
	switch x := r.X.(type) {
	case *ast.CompositeLit:
		return isSliceOrMapType(x.Type)
	case *ast.Ident:
		return tableIdents[x.Name]
	}
	return false
}

// bodyUsesT reports whether the range body drives testing through a t-var:
// a subtest, an assertion helper taking t, or a direct t.Errorf-style call.
func bodyUsesT(body *ast.BlockStmt, tvars map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && tvars[x.Name] {
				found = true
				return false
			}
		}
		for _, arg := range call.Args {
			if id, ok := arg.(*ast.Ident); ok && tvars[id.Name] {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// collectTParams gathers names of params typed *testing.T, *testing.B, or
// testing.TB from every FuncDecl and FuncLit in the file.
func collectTParams(f *ast.File) map[string]bool {
	out := map[string]bool{}
	addFields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, field := range fl.List {
			if !isTestingType(field.Type) {
				continue
			}
			for _, name := range field.Names {
				out[name.Name] = true
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			addFields(n.Type.Params)
		case *ast.FuncLit:
			addFields(n.Type.Params)
		}
		return true
	})
	return out
}

func isTestingType(e ast.Expr) bool {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "testing" {
		return false
	}
	switch sel.Sel.Name {
	case "T", "B", "TB", "F":
		return true
	}
	return false
}

func isTestSig(fd *ast.FuncDecl, kind string) bool {
	if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
		return false
	}
	star, ok := fd.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "testing" && sel.Sel.Name == kind
}

// collectDurConsts resolves same-file const/var duration declarations so
// time.Sleep(pollInterval) still yields a known duration.
func collectDurConsts(f *ast.File) map[string]int64 {
	out := map[string]int64{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != len(vs.Values) {
			return true
		}
		for i, v := range vs.Values {
			if ms, ok := durMS(v, nil); ok {
				out[vs.Names[i].Name] = ms
			}
		}
		return true
	})
	return out
}

func sleepMS(call *ast.CallExpr, durs map[string]int64) (int64, bool) {
	if len(call.Args) != 1 {
		return 0, false
	}
	return durMS(call.Args[0], durs)
}

var unitMS = map[string]float64{
	"Nanosecond": 1e-6, "Microsecond": 1e-3, "Millisecond": 1,
	"Second": 1000, "Minute": 60000, "Hour": 3600000,
}

func durMS(e ast.Expr, durs map[string]int64) (int64, bool) {
	switch e := e.(type) {
	case *ast.Ident:
		if durs != nil {
			ms, ok := durs[e.Name]
			return ms, ok
		}
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && x.Name == "time" {
			if ms, ok := unitMS[e.Sel.Name]; ok {
				return int64(ms), true
			}
		}
	case *ast.BinaryExpr:
		if e.Op != token.MUL {
			return 0, false
		}
		if n, ok := litFloat(e.X); ok {
			if ms, ok2 := durMS(e.Y, durs); ok2 {
				return int64(n * float64(ms)), true
			}
		}
		if n, ok := litFloat(e.Y); ok {
			if ms, ok2 := durMS(e.X, durs); ok2 {
				return int64(n * float64(ms)), true
			}
		}
	}
	return 0, false
}

func litFloat(e ast.Expr) (float64, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || (lit.Kind != token.INT && lit.Kind != token.FLOAT) {
		return 0, false
	}
	v, err := strconv.ParseFloat(lit.Value, 64)
	return v, err == nil
}

func isGenerated(src []byte) bool {
	for _, line := range strings.SplitN(string(src[:min(len(src), 2048)]), "\n", 30) {
		if generatedRe.MatchString(strings.TrimSpace(line)) {
			return true
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return false
}

func hasIntegrationTag(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:build") && strings.Contains(c.Text, "integration") {
				return true
			}
		}
	}
	return false
}

func readModulePath(gomod string) string {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func scanComposeFile(path string, st *repoStats) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if m := composeImageRe.FindStringSubmatch(line); m != nil && plausibleImage(m[1]) {
			st.ComposeImages[m[1]]++
		}
	}
}

func packMap(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s:%d", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func writeCSV(path string, all []*repoStats) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	w := csv.NewWriter(fh)
	defer w.Flush()
	head := []string{"repo", "sha", "test_files", "test_loc", "nontest_loc", "pkgs_with_tests",
		"test_funcs", "ginkgo_specs", "fuzz_funcs", "bench_funcs", "testmain_pkgs",
		"subtest_calls", "subtest_max_depth", "parallel_tests", "parallel_subtests",
		"table_tests", "underscore_names", "sleep_calls", "sleep_known_ms",
		"cleanup_calls", "helper_calls", "skip_calls", "short_guards",
		"golden_files", "testdata_refs", "update_flag", "golden_practice",
		"container_pkgs", "testmain_tc_pkgs", "tc_modules", "container_images", "compose_images",
		"int_build_tag_files", "frameworks"}
	if err := w.Write(head); err != nil {
		return err
	}
	for _, s := range all {
		row := []string{s.Name, s.SHA,
			itoa(s.TestFiles), itoa(s.TestLOC), itoa(s.NonTestLOC), itoa(s.PkgsWithTests),
			itoa(s.TestFuncs), itoa(s.GinkgoSpecs), itoa(s.FuzzFuncs), itoa(s.BenchFuncs), itoa(s.TestMainPkgs),
			itoa(s.SubtestCalls), itoa(s.SubtestMaxD), itoa(s.ParallelTests), itoa(s.ParallelSubs),
			itoa(s.TableTests), itoa(s.UnderscoreName), itoa(s.SleepCalls),
			strconv.FormatInt(s.SleepKnownMS, 10),
			itoa(s.CleanupCalls), itoa(s.HelperCalls), itoa(s.SkipCalls), itoa(s.ShortGuards),
			itoa(s.GoldenFiles), itoa(s.TestdataRefs), strconv.FormatBool(s.UpdateFlag), strconv.FormatBool(s.GoldenPractice),
			itoa(s.ContainerPkgs), itoa(s.TestMainTCPkgs), packMap(s.TCModules), packMap(s.ContainerImages), packMap(s.ComposeImages),
			itoa(s.BuildTagIntTests), packMap(s.Frameworks)}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func itoa(i int) string { return strconv.Itoa(i) }

func summarize(all []*repoStats) {
	n := len(all)
	if n == 0 {
		return
	}
	var tests, par, parSubs, subtests, table, sleeps, cleanup, helper, underscore, fuzz, golden, ginkgo int
	var sleepMS int64
	fwRepos := map[string]int{}
	tcMods := map[string]int{}   // module -> repos
	tcImages := map[string]int{} // normalized image -> repos (code + compose)
	var updateRepos, goldenPractRepos, tdRefRepos, tcRepos, tcMultiPkg, sleepRepos, parRepos, fuzzRepos, goldenRepos, tmRepos, deepSubRepos int
	for _, s := range all {
		tests += s.TestFuncs
		par += s.ParallelTests
		parSubs += s.ParallelSubs
		subtests += s.SubtestCalls
		table += s.TableTests
		sleeps += s.SleepCalls
		sleepMS += s.SleepKnownMS
		cleanup += s.CleanupCalls
		helper += s.HelperCalls
		underscore += s.UnderscoreName
		fuzz += s.FuzzFuncs
		golden += s.GoldenFiles
		ginkgo += s.GinkgoSpecs
		seen := map[string]bool{}
		for fw := range s.Frameworks {
			if !seen[fw] {
				fwRepos[fw]++
				seen[fw] = true
			}
		}
		seenM := map[string]bool{}
		for m := range s.TCModules {
			if !seenM[m] {
				tcMods[m]++
				seenM[m] = true
			}
		}
		seenI := map[string]bool{}
		for img := range s.ContainerImages {
			base := strings.SplitN(img, ":", 2)[0]
			if !seenI[base] {
				tcImages[base]++
				seenI[base] = true
			}
		}
		for img := range s.ComposeImages {
			base := strings.SplitN(img, ":", 2)[0]
			if !seenI[base] {
				tcImages[base]++
				seenI[base] = true
			}
		}
		if s.UpdateFlag {
			updateRepos++
		}
		if s.GoldenPractice {
			goldenPractRepos++
		}
		if s.TestdataRefs > 0 {
			tdRefRepos++
		}
		if s.Frameworks["testcontainers"] > 0 || s.Frameworks["dockertest"] > 0 || s.ContainerPkgs > 0 {
			tcRepos++
		}
		if s.TestMainTCPkgs > 1 {
			tcMultiPkg++
		}
		if s.SleepCalls > 0 {
			sleepRepos++
		}
		if s.ParallelTests > 0 || s.ParallelSubs > 0 {
			parRepos++
		}
		if s.FuzzFuncs > 0 {
			fuzzRepos++
		}
		if s.GoldenFiles > 0 {
			goldenRepos++
		}
		if s.TestMainPkgs > 0 {
			tmRepos++
		}
		if s.SubtestMaxD >= 2 {
			deepSubRepos++
		}
	}
	pct := func(a, b int) string { return fmt.Sprintf("%.1f%%", 100*float64(a)/float64(max(b, 1))) }
	fmt.Printf("\n===== CENSUS SUMMARY (%d repos, %d test funcs, %d ginkgo specs) =====\n", n, tests, ginkgo)
	fmt.Printf("t.Parallel():     %s of tests are parallel themselves | %d parallel subtest closures (%s of subtests) | %s of repos\n",
		pct(par, tests), parSubs, pct(parSubs, subtests), pct(parRepos, n))
	fmt.Printf("table-driven:     %s of tests\n", pct(table, tests))
	fmt.Printf("subtests:         %d calls (%.2f per test) | max nesting >=2 in %s of repos\n",
		subtests, float64(subtests)/float64(max(tests, 1)), pct(deepSubRepos, n))
	fmt.Printf("underscore names: %s of tests\n", pct(underscore, tests))
	fmt.Printf("time.Sleep:       %d calls in %s of repos | known const duration total: %.1f min\n",
		sleeps, pct(sleepRepos, n), float64(sleepMS)/60000)
	fmt.Printf("t.Cleanup():      %d calls | t.Helper(): %d calls | fuzz: %d funcs in %s of repos\n",
		cleanup, helper, fuzz, pct(fuzzRepos, n))
	fmt.Printf("TestMain:         %s of repos\n", pct(tmRepos, n))
	fmt.Printf("golden/testdata:  %d .golden files (%s of repos) | testdata refs in %s | update flag in %s | full golden practice in %s\n",
		golden, pct(goldenRepos, n), pct(tdRefRepos, n), pct(updateRepos, n), pct(goldenPractRepos, n))
	fmt.Printf("containers:       %s of repos | multi-pkg TestMain containers: %d repos\n", pct(tcRepos, n), tcMultiPkg)
	if len(tcMods) > 0 {
		fmt.Println("testcontainers modules (repos):", sortedKV(tcMods))
	}
	if len(tcImages) > 0 {
		fmt.Println("container images (repos, code+compose):", sortedKV(tcImages))
	}
	fmt.Println("framework adoption (share of repos):")
	keys := make([]string, 0, len(fwRepos))
	for k := range fwRepos {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return fwRepos[keys[i]] > fwRepos[keys[j]] })
	for _, k := range keys {
		fmt.Printf("  %-16s %s\n", k, pct(fwRepos[k], n))
	}
}

func sortedKV(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, " ")
}
