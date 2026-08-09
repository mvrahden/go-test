package gotestgen_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const brokenloadPkgBase = "github.com/mvrahden/go-test/internal/gotestgen/testdata/brokenload"

// brokenloadDir returns the absolute path of the testdata/brokenload fixture
// tree. The fixtures live under testdata so the go tool's pattern expansion
// never picks them up as part of the module's own build.
func brokenloadDir(t *gotest.T) string {
	cwd, err := os.Getwd()
	gotest.NoError(t, err)
	return filepath.Join(cwd, "testdata", "brokenload")
}

// BrokenLoadTestSuite exercises the loader's broken-package verdicts. Every
// pattern-matched package must end in exactly one verdict: a package that
// fails to parse or type-check surfaces as a BrokenPackage entry and is
// excluded from the loaded results, in every variant.
type BrokenLoadTestSuite struct{}

func (s *BrokenLoadTestSuite) TestBrokenPackageVerdicts(t *gotest.T) {
	loaded, broken, err := gotestgen.LoadPackages([]string{filepath.Join(brokenloadDir(t), "...")}, nil)
	gotest.NoError(t, err)

	t.It("keeps only fully loadable packages in the loaded results", func(it *gotest.T) {
		gotest.Len(it, loaded, 1)
		gotest.Equal(it, brokenloadPkgBase+"/healthy", loaded[0].PkgPath)
	})

	t.It("returns one broken entry per unbuildable package, sorted by path", func(it *gotest.T) {
		paths := make([]string, 0, len(broken))
		for i := range broken {
			paths = append(paths, broken[i].PkgPath)
		}
		gotest.Equal(it, []string{
			brokenloadPkgBase + "/brokensyntax",
			brokenloadPkgBase + "/brokentest",
			brokenloadPkgBase + "/brokentype",
			brokenloadPkgBase + "/brokenxtest",
		}, paths)
	})

	t.It("carries each distinct diagnostic exactly once", func(it *gotest.T) {
		for i := range broken {
			gotest.NotEmpty(it, broken[i].Errors, "%s must carry its diagnostics", broken[i].PkgPath)
			seen := map[string]bool{}
			for _, msg := range broken[i].Errors {
				gotest.False(it, seen[msg], "duplicate diagnostic for %s: %s", broken[i].PkgPath, msg)
				seen[msg] = true
			}
		}
	})

	t.It("reports the type error with its position", func(it *gotest.T) {
		var msgs []string
		for i := range broken {
			if broken[i].PkgPath == brokenloadPkgBase+"/brokentype" {
				msgs = broken[i].Errors
			}
		}
		gotest.True(it, len(msgs) > 0 && strings.Contains(strings.Join(msgs, "\n"), "cannot use"),
			"expected a compiler diagnostic, got %v", msgs)
	})
}

func (s *BrokenLoadTestSuite) TestBrokenVariantExcludesPackage(t *gotest.T) {
	t.When("only the external test variant is broken", func(w *gotest.T) {
		loaded, broken, err := gotestgen.LoadPackages([]string{filepath.Join(brokenloadDir(t), "brokenxtest")}, nil)
		gotest.NoError(w, err)

		w.It("books the package as broken", func(it *gotest.T) {
			gotest.Len(it, broken, 1)
			gotest.Equal(it, brokenloadPkgBase+"/brokenxtest", broken[0].PkgPath)
		})

		w.It("does not run the intact variants", func(it *gotest.T) {
			// The test build for the package fails as a whole; a partial run
			// of the intact internal variant would misreport the package.
			gotest.Empty(it, loaded)
		})
	})
}

func (s *BrokenLoadTestSuite) TestDiscoveryLoaderAgreesOnBroken(t *gotest.T) {
	t.It("returns the same broken packages as the full loader", func(it *gotest.T) {
		_, broken, err := gotestgen.LoadPackagesForDiscovery([]string{filepath.Join(brokenloadDir(t), "brokensyntax")}, nil)
		gotest.NoError(it, err)
		gotest.Len(it, broken, 1)
		gotest.Equal(it, brokenloadPkgBase+"/brokensyntax", broken[0].PkgPath)
	})
}
