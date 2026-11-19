package perf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
)

// stage writes the synthetic module at dir: p packages g/pNN, each with s
// suites of k methods, and their stdlib twins s/pNN, each with s tests of k
// subtests of the same body.
func stage(repoRoot, dir string, p, s, k int) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := testkit.WriteModule(repoRoot, dir, "perf.example"); err != nil {
		return err
	}
	for i := range p {
		name := fmt.Sprintf("p%02d", i)
		for tree, tests := range map[string]string{"g": gotestFile(name, s, k), "s": stdlibFile(name, s, k)} {
			pkg := filepath.Join(dir, tree, name)
			if err := os.MkdirAll(pkg, 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(pkg, "pkg.go"), []byte(sourceFile(name)), 0o600); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(pkg, "suites_test.go"), []byte(tests), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// sourceFile is a staged package's code; edit rewrites its marker.
func sourceFile(name string) string {
	return "package " + name + "\n\nvar editMarker = 0\n\nfunc Add(a, b int) int { return a + b }\n"
}

// gotestFile holds s suites SxxTestSuite of k methods TestMyy.
func gotestFile(name string, s, k int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\nvar editMarker_test = 0\n", name)
	for i := range s {
		fmt.Fprintf(&b, "\ntype S%02dTestSuite struct{}\n", i)
		for j := range k {
			fmt.Fprintf(&b, "\nfunc (s *S%02dTestSuite) TestM%02d(t *gotest.T) { gotest.Equal(t, %d, Add(%d, 2)) }\n", i, j, j+2, j)
		}
	}
	return b.String()
}

// stdlibFile holds s tests TestSxx of k subtests, the twin of gotestFile.
func stdlibFile(name string, s, k int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport \"testing\"\n\nvar editMarker_test = 0\n", name)
	for i := range s {
		fmt.Fprintf(&b, "\nfunc TestS%02d(t *testing.T) {\n", i)
		for j := range k {
			fmt.Fprintf(&b, "\tt.Run(\"M%02d\", func(t *testing.T) {\n\t\tif got := Add(%d, 2); got != %d {\n\t\t\tt.Fatalf(\"got %%d\", got)\n\t\t}\n\t})\n", j, j, j+2)
		}
		b.WriteString("}\n")
	}
	return b.String()
}

var editMarkerLine = regexp.MustCompile(`(?m)^(var editMarker(?:_test)? = )-?\d+$`)

// edit rewrites the editMarker of file with a value never used before, so
// the package's build action key changes (a comment would not).
func edit(file string) error {
	data, err := os.ReadFile(file) //nolint:gosec // G304: a file of the staged module
	if err != nil {
		return err
	}
	if !editMarkerLine.Match(data) {
		return fmt.Errorf("%s has no editMarker line", file)
	}
	next := editMarkerLine.ReplaceAll(data, []byte("${1}"+strconv.FormatInt(time.Now().UnixNano(), 10)))
	return os.WriteFile(file, next, 0o600)
}
