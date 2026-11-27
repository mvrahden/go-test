package inputhash_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/inputhash"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// InputHashTestSuite: what a package's hash moves with, and what leaves it
// as it is.
type InputHashTestSuite struct{}

func (s *InputHashTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second} // a few go list calls
}

// module writes a module m: a imports b, a/sub sits under a/testdata's
// directory, c stands alone.
func module(t *gotest.T) string {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                       "module m\n\ngo 1.24\n",
		"a/a.go":                       "package a\n\nimport \"m/b\"\n\nvar A = b.B\n",
		"a/a_test.go":                  "package a\n",
		"a/gotest_psuite_test.go":      "package a\n",
		"a/testdata/x.txt":             "x\n",
		"a/sub/sub.go":                 "package sub\n",
		"b/b.go":                       "package b\n\nvar B = 1\n",
		"c/c.go":                       "package c\n",
		"c/README.md":                  "not read\n",
		"c/testdata/nested/deep/y.txt": "y\n",
	}
	for rel, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		gotest.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	}
	return dir
}

func hashes(t *gotest.T, dir string, flags ...string) inputhash.Hashes {
	h, err := inputhash.Current(t.Context(), dir, []string{"./..."}, flags, flags)
	gotest.NoError(t, err)
	gotest.Len(t, h, 4)
	return h
}

// changedBy is the packages whose hash an edit of rel moves.
func changedBy(t *gotest.T, rel, text string) []string {
	dir := module(t)
	before := hashes(t, dir)
	path := filepath.Join(dir, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(path, append(data, text...), 0o600))
	after := hashes(t, dir)
	var out []string
	for _, pkg := range []string{"m/a", "m/a/sub", "m/b", "m/c"} {
		if before[pkg] != after[pkg] {
			out = append(out, pkg)
		}
	}
	return out
}

func (s *InputHashTestSuite) TestWhatMovesAHash(t *gotest.T) {
	t.When("one file of the module changes", func(w *gotest.T) {
		w.It("moves the hash of each package that reads it, and of no other", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc    string
				rel     string
				changed []string
			}{
				{Desc: "a dependency's file", rel: "b/b.go", changed: []string{"m/a", "m/b"}},
				{Desc: "a test file", rel: "a/a_test.go", changed: []string{"m/a"}},
				{Desc: "a testdata file above the package", rel: "a/testdata/x.txt", changed: []string{"m/a", "m/a/sub"}},
				{Desc: "a nested testdata file", rel: "c/testdata/nested/deep/y.txt", changed: []string{"m/c"}},
				{Desc: "a comment in go.mod", rel: "go.mod", changed: nil},
				{Desc: "a file no package reads", rel: "c/README.md", changed: nil},
				{Desc: "a generated file", rel: "a/gotest_psuite_test.go", changed: nil},
			}) {
				gotest.Equal(sub, r.changed, changedBy(sub, r.rel, "\n// edited\n"))
			}
		})
		w.It("moves every hash when the go line changes", func(it *gotest.T) {
			dir := module(it)
			before := hashes(it, dir)
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.25\n"), 0o600))
			after := hashes(it, dir)
			for pkg, h := range before {
				gotest.NotEqual(it, h, after[pkg], pkg)
			}
		})
	})
}

func (s *InputHashTestSuite) TestPlacesAndFlags(t *gotest.T) {
	t.When("the same tree is hashed in two places", func(w *gotest.T) {
		w.It("gives each package the same hash", func(it *gotest.T) {
			gotest.Equal(it, hashes(it, module(it)), hashes(it, module(it)))
		})
	})
	t.When("the build flags differ", func(w *gotest.T) {
		w.It("moves the hashes for a flag that changes the build and not for one that changes what go prints", func(it *gotest.T) {
			dir := module(it)
			plain := hashes(it, dir)
			gotest.Equal(it, plain, hashes(it, dir, "-x", "-p=2"))
			gotest.NotEqual(it, plain["m/c"], hashes(it, dir, "-tags=extra")["m/c"])
		})
		w.It("normalizes each flag with its value into one word, sorted", func(it *gotest.T) {
			gotest.Equal(it, []string{"-gcflags=-N -l", "-race", "-tags=a,b"}, inputhash.Flags([]string{"-tags", "a,b", "-v", "-race", "-p", "4", "-gcflags", "-N -l", "-x"}))
		})
	})
}
