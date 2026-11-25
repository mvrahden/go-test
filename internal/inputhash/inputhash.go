// Package inputhash digests what a package's test verdict depends on as
// source, so a verdict can be told current or stale against a tree.
//
// A package's hash covers, as one sorted list of lines: each file of the
// package and of every package it reaches in a main module (test variants
// and test main included, gotest's generated files aside), the files they
// embed, the testdata trees of the package's directory and of each above it
// in its module, the module version of every other package reached, the
// go, toolchain and godebug lines of each main module's go.mod and of the
// go.work, and the build settings. Paths are module-relative, so two
// checkouts of one tree hash alike. Nothing a test reads at run time
// outside those files is seen.
package inputhash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/about"
)

// Hashes maps a package's import path to its input hash.
type Hashes map[string]string

// Packages hashes every package the graph's roots name, under settings.
// Roots are what a load with tests returns: a package, its test variants
// and its test main, grouped by the package's import path.
func Packages(roots []*packages.Package, settings Settings) Hashes {
	h := &hasher{files: map[string]string{}, trees: map[string]string{}, mods: map[string][]string{}, nodes: map[*packages.Package][]string{}}
	groups := map[string][]*packages.Package{}
	var order []string
	for _, p := range roots {
		path := PackageOf(p)
		if _, ok := groups[path]; !ok {
			order = append(order, path)
		}
		groups[path] = append(groups[path], p)
	}
	build := settings.lines()
	out := make(Hashes, len(order))
	var mu sync.Mutex
	parallel(len(order), func(i int) {
		path := order[i]
		sum := h.hash(groups[path], build)
		mu.Lock()
		out[path] = sum
		mu.Unlock()
	})
	return out
}

// PackageOf is the import path a root stands for: a test variant's or a
// test main's package.
func PackageOf(p *packages.Package) string {
	path := strings.TrimSuffix(p.PkgPath, ".test")
	return strings.TrimSuffix(path, "_test")
}

type hasher struct {
	mu    sync.Mutex
	files map[string]string   // absolute path → content digest
	trees map[string]string   // testdata dir → tree digest
	mods  map[string][]string // go.mod or go.work path → its lines that matter
	nodes map[*packages.Package][]string
}

// hash is the input hash of one package's roots.
func (h *hasher) hash(roots []*packages.Package, build []string) string {
	lines := map[string]bool{}
	seen := map[string]bool{}
	var walk func(p *packages.Package)
	walk = func(p *packages.Package) {
		if seen[p.ID] {
			return
		}
		seen[p.ID] = true
		if p.Module == nil {
			// The standard library, which the Go version stands for.
			return
		}
		for _, l := range h.own(p) {
			lines[l] = true
		}
		for _, dep := range p.Imports {
			walk(dep)
		}
	}
	for _, p := range roots {
		walk(p)
	}
	for _, p := range roots {
		if p.Module == nil || !local(p.Module) {
			continue
		}
		for _, dir := range testdataDirs(p) {
			lines["testdata "+modRel(p.Module, dir)+" "+h.tree(dir)] = true
		}
	}
	for _, l := range build {
		lines["build "+l] = true
	}
	sorted := make([]string, 0, len(lines))
	for l := range lines {
		sorted = append(sorted, l)
	}
	slices.Sort(sorted)
	sum := sha256.New()
	for _, l := range sorted {
		sum.Write([]byte(l))
		sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil))[:32]
}

// own are the lines a package adds for itself: its files, or its module's
// version.
func (h *hasher) own(p *packages.Package) []string {
	h.mu.Lock()
	lines, ok := h.nodes[p]
	h.mu.Unlock()
	if ok {
		return lines
	}
	m := p.Module
	if !local(m) {
		lines = []string{"module " + moduleVersion(m)}
	} else {
		for _, f := range slices.Concat(p.GoFiles, p.OtherFiles, p.EmbedFiles) {
			if about.PSuiteRegex.MatchString(f) {
				continue
			}
			lines = append(lines, "file "+modRel(m, f)+" "+h.file(f))
		}
		if m.GoMod != "" {
			for _, l := range h.mod(m.GoMod, false) {
				lines = append(lines, "gomod "+m.Path+" "+l)
			}
		}
	}
	h.mu.Lock()
	h.nodes[p] = lines
	h.mu.Unlock()
	return lines
}

// local reports whether a module's source is in the tree: a main module, a
// go.work one included, or one a replace points at a directory.
func local(m *packages.Module) bool {
	return m.Main || m.Replace != nil && m.Replace.Version == ""
}

func moduleVersion(m *packages.Module) string {
	s := m.Path + "@" + m.Version
	if r := m.Replace; r != nil {
		s += " => " + r.Path + "@" + r.Version
	}
	return s
}

// modRel is path as the module names it: its path, then the file's place
// in its directory.
func modRel(m *packages.Module, path string) string {
	dir := m.Dir
	if m.Replace != nil && m.Replace.Dir != "" {
		dir = m.Replace.Dir
	}
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return m.Path + "/" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// testdataDirs are the testdata directories of p's directory and of each
// directory above it in its module.
func testdataDirs(p *packages.Package) []string {
	dir := p.Dir
	if dir == "" && len(p.GoFiles) > 0 {
		dir = filepath.Dir(p.GoFiles[0])
	}
	root := p.Module.Dir
	if dir == "" || root == "" {
		return nil
	}
	var out []string
	for {
		td := filepath.Join(dir, "testdata")
		if fi, err := os.Stat(td); err == nil && fi.IsDir() {
			out = append(out, td)
		}
		if rel, err := filepath.Rel(root, dir); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return out
		}
		dir = filepath.Dir(dir)
	}
}

// file is the digest of a file's bytes; one that cannot be read digests
// as missing.
func (h *hasher) file(path string) string {
	h.mu.Lock()
	sum, ok := h.files[path]
	h.mu.Unlock()
	if ok {
		return sum
	}
	sum = digestFile(path)
	h.mu.Lock()
	h.files[path] = sum
	h.mu.Unlock()
	return sum
}

func digestFile(path string) string {
	f, err := os.Open(path) //nolint:gosec // G304: a file of the package the graph lists
	if err != nil {
		return "missing"
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "unreadable"
	}
	return hex.EncodeToString(sum.Sum(nil))[:32]
}

// tree is the digest of every file under dir, by its place and its bytes.
func (h *hasher) tree(dir string) string {
	h.mu.Lock()
	sum, ok := h.trees[dir]
	h.mu.Unlock()
	if ok {
		return sum
	}
	all := sha256.New()
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		all.Write([]byte(filepath.ToSlash(rel) + " " + digestFile(path) + "\n"))
		return nil
	})
	sum = hex.EncodeToString(all.Sum(nil))[:32]
	h.mu.Lock()
	h.trees[dir] = sum
	h.mu.Unlock()
	return sum
}

// mod is the lines of a go.mod or go.work a build reads beside the module
// versions: go, toolchain and godebug.
func (h *hasher) mod(path string, work bool) []string {
	h.mu.Lock()
	lines, ok := h.mods[path]
	h.mu.Unlock()
	if ok {
		return lines
	}
	lines = modLines(path, work)
	h.mu.Lock()
	h.mods[path] = lines
	h.mu.Unlock()
	return lines
}

func modLines(path string, work bool) []string {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the module's own go.mod or go.work
	if err != nil {
		return []string{"missing"}
	}
	var goLine, toolchain string
	var godebug []*modfile.Godebug
	if work {
		f, err := modfile.ParseWork(path, data, nil)
		if err != nil {
			return []string{"unparsed " + digestBytes(data)}
		}
		if f.Go != nil {
			goLine = f.Go.Version
		}
		if f.Toolchain != nil {
			toolchain = f.Toolchain.Name
		}
		godebug = f.Godebug
	} else {
		f, err := modfile.ParseLax(path, data, nil)
		if err != nil {
			return []string{"unparsed " + digestBytes(data)}
		}
		if f.Go != nil {
			goLine = f.Go.Version
		}
		if f.Toolchain != nil {
			toolchain = f.Toolchain.Name
		}
		godebug = f.Godebug
	}
	lines := []string{"go " + goLine}
	if toolchain != "" {
		lines = append(lines, "toolchain "+toolchain)
	}
	for _, g := range godebug {
		lines = append(lines, "godebug "+g.Key+"="+g.Value)
	}
	return lines
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:32]
}

// parallel runs f(0..n-1) on a few goroutines.
func parallel(n int, f func(i int)) {
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(n, 8) {
		wg.Go(func() {
			for i := range work {
				f(i)
			}
		})
	}
	for i := range n {
		work <- i
	}
	close(work)
	wg.Wait()
}
