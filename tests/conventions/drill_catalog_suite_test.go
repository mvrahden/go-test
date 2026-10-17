package conventions_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DrillCatalogTestSuite pins tests/drill/README.md to tests/drill/mutants:
// every patch is listed, every listed name has a patch, and every patch
// names the check that must catch it. A mutant the README does not mention
// is a guard nobody knows exists; a row without a patch guards nothing.
type DrillCatalogTestSuite struct{}

// catalogRowRe matches a table row whose first cell is a backticked mutant name.
var catalogRowRe = regexp.MustCompile("(?m)^\\| `([a-z0-9-]+)` \\|")

func (s *DrillCatalogTestSuite) TestEveryMutantIsCatalogued(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	drill := filepath.Join(root, "tests", "drill")

	patchFiles, err := filepath.Glob(filepath.Join(drill, "mutants", "*.patch"))
	gotest.NoError(t, err)
	gotest.NotEmpty(t, patchFiles)
	var patches []string
	for _, f := range patchFiles {
		patches = append(patches, strings.TrimSuffix(filepath.Base(f), ".patch"))
	}

	readme, err := os.ReadFile(filepath.Join(drill, "README.md"))
	gotest.NoError(t, err)
	var listed []string
	for _, m := range catalogRowRe.FindAllStringSubmatch(string(readme), -1) {
		listed = append(listed, m[1])
	}

	t.When("the mutant directory and the README table are compared", func(w *gotest.T) {
		w.It("lists exactly the patches that exist", func(it *gotest.T) {
			gotest.ElementsMatch(it, patches, listed)
		})
	})

	t.When("each patch is read", func(w *gotest.T) {
		w.It("names the check that must catch it on an Expect: line", func(it *gotest.T) {
			for _, f := range patchFiles {
				data, err := os.ReadFile(f)
				gotest.NoError(it, err)
				gotest.Regexp(it, "(?m)^Expect: .+", string(data), "%s has no Expect: line", filepath.Base(f))
			}
		})
	})
}
