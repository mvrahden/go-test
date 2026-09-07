package gotestgen_test

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// RuntimeVersionTestSuite covers the preflight that refuses to generate
// against a gotest runtime older than the renderer targets.
type RuntimeVersionTestSuite struct{}

func (s *RuntimeVersionTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func loadedWith(mod *packages.Module, external bool) []*gotestgen.LoadResult {
	pkg := &packages.Package{
		PkgPath: "example.com/app",
		Imports: map[string]*packages.Package{
			about.Repo + "/pkg/gotest": {PkgPath: about.Repo + "/pkg/gotest", Module: mod},
			"fmt":                      {PkgPath: "fmt"},
		},
	}
	lr := &gotestgen.LoadResult{PkgPath: "example.com/app"}
	if external {
		lr.Pxtest = pkg
	} else {
		lr.Ptest = pkg
	}
	return []*gotestgen.LoadResult{lr}
}

func (s *RuntimeVersionTestSuite) TestCheckRuntimeVersion(t *gotest.T) {
	t.When("the module pins a runtime older than the generator's floor", func(w *gotest.T) {
		w.It("fails with the pinned version, the floor and the upgrade command", func(it *gotest.T) {
			err := gotestgen.CheckRuntimeVersion(loadedWith(&packages.Module{Path: about.Repo, Version: "v1.25.0"}, false))
			gotest.ErrorContains(it, err, "v1.25.0")
			gotest.ErrorContains(it, err, gotestgen.MinRuntimeVersion)
			gotest.ErrorContains(it, err, "go get -tool "+about.Repo+"/cmd/gotest@latest")
		})
		w.It("checks external test packages too", func(it *gotest.T) {
			err := gotestgen.CheckRuntimeVersion(loadedWith(&packages.Module{Path: about.Repo, Version: "v1.25.0"}, true))
			gotest.Error(it, err)
		})
		w.It("treats a pseudo-version by its base version", func(it *gotest.T) {
			err := gotestgen.CheckRuntimeVersion(loadedWith(&packages.Module{Path: about.Repo, Version: "v1.25.1-0.20260701120000-abcdefabcdef"}, false))
			gotest.Error(it, err)
		})
	})

	t.When("the module pins a runtime the generator can target", func(w *gotest.T) {
		w.It("accepts the floor itself", func(it *gotest.T) {
			gotest.NoError(it, gotestgen.CheckRuntimeVersion(loadedWith(&packages.Module{Path: about.Repo, Version: gotestgen.MinRuntimeVersion}, false)))
		})
		w.It("accepts newer releases", func(it *gotest.T) {
			gotest.NoError(it, gotestgen.CheckRuntimeVersion(loadedWith(&packages.Module{Path: about.Repo, Version: "v1.99.0"}, false)))
		})
	})

	t.When("the version says nothing about the code that will run", func(w *gotest.T) {
		w.It("skips a replaced module", func(it *gotest.T) {
			mod := &packages.Module{Path: about.Repo, Version: "v0.0.0-00010101000000-000000000000", Replace: &packages.Module{Path: "../go-test"}}
			gotest.NoError(it, gotestgen.CheckRuntimeVersion(loadedWith(mod, false)))
		})
		w.It("skips the main module", func(it *gotest.T) {
			gotest.NoError(it, gotestgen.CheckRuntimeVersion(loadedWith(&packages.Module{Path: about.Repo, Main: true}, false)))
		})
		w.It("skips packages that do not import gotest", func(it *gotest.T) {
			lr := &gotestgen.LoadResult{PkgPath: "example.com/plain", Ptest: &packages.Package{PkgPath: "example.com/plain"}}
			gotest.NoError(it, gotestgen.CheckRuntimeVersion([]*gotestgen.LoadResult{lr}))
		})
	})
}

func (s *RuntimeVersionTestSuite) TestFloorMatchesExtension(t *gotest.T) {
	t.It("equals MIN_CLI_VERSION in vscode-gotest/src/cli.ts", func(it *gotest.T) {
		src, err := os.ReadFile(filepath.Join("..", "..", "vscode-gotest", "src", "cli.ts"))
		gotest.NoError(it, err)
		m := regexp.MustCompile(`MIN_CLI_VERSION = "(v[^"]+)"`).FindSubmatch(src)
		gotest.NotNil(it, m)
		gotest.Equal(it, string(m[1]), gotestgen.MinRuntimeVersion)
	})
}
