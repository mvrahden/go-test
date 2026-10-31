package scaffold_test

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/scaffold"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScaffoldCompilesTestSuite scaffolds every target shape into a copy of a
// testdata package and type-checks the result with go vet.
type ScaffoldCompilesTestSuite struct{}

// SuiteConfig: each row builds a module against this repository.
func (s *ScaffoldCompilesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// stagePackage copies testdata/<name> into a fresh module and returns the
// package's absolute directory there.
func stagePackage(t *gotest.T, name string) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	modDir := t.TempDir()
	gotest.NoError(t, testkit.WriteModule(root, modDir, "example.com/shapes"))
	pkgDir := filepath.Join(modDir, name)
	gotest.NoError(t, os.MkdirAll(pkgDir, 0o750))
	srcs, err := filepath.Glob(filepath.Join("testdata", name, "*.go"))
	gotest.NoError(t, err)
	for _, src := range srcs {
		data, err := os.ReadFile(src)
		gotest.NoError(t, err)
		gotest.NoError(t, os.WriteFile(filepath.Join(pkgDir, filepath.Base(src)), data, 0o600))
	}
	return pkgDir
}

type shapeScaffold func(t *gotest.T, pkgDir string) (filename string, src []byte)

func typeShape(typeName string) shapeScaffold {
	return func(t *gotest.T, pkgDir string) (string, []byte) {
		info, err := scaffold.IntrospectType(pkgDir, typeName)
		gotest.NoError(t, err)
		var out []byte
		if info.IsInterface {
			out, err = scaffold.GenerateContractScaffold(info)
		} else {
			out, err = scaffold.GenerateScaffold(info)
		}
		gotest.NoError(t, err)
		return scaffold.ToSnakeCase(typeName) + "_suite_test.go", out
	}
}

func fileShape(filename string) shapeScaffold {
	return func(t *gotest.T, pkgDir string) (string, []byte) {
		info, err := scaffold.IntrospectFile(pkgDir, filename)
		gotest.NoError(t, err)
		out, err := scaffold.GenerateFileScaffold(info)
		gotest.NoError(t, err)
		return "file_suite_test.go", out
	}
}

func fuzzShape(funcName string) shapeScaffold {
	return func(t *gotest.T, pkgDir string) (string, []byte) {
		target, err := scaffold.IntrospectFuzzTarget(pkgDir, funcName)
		gotest.NoError(t, err)
		out, _, err := scaffold.GenerateFuzzScaffold(target)
		gotest.NoError(t, err)
		return scaffold.ToSnakeCase(funcName) + "_fuzz_test.go", out
	}
}

func (s *ScaffoldCompilesTestSuite) TestEveryShapeCompiles(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		pkg      string
		scaffold shapeScaffold
	}{
		{"a type with methods", "sampletype", typeShape("UserService")},
		{"an interface", "sampletype", typeShape("Validator")},
		{"a file of functions", "sampletype", fileShape("funcs.go")},
		{"a file of generic functions", "generic", fileShape("generic.go")},
		{"a function to fuzz", "sampletype", fuzzShape("Encode")},
		{"a function whose parameter cannot be fuzzed", "sampletype", fuzzShape("ApplyOptions")},
		{"a type beside generic ones", "generic", typeShape("Plain")},
	}) {
		pkgDir := stagePackage(sub, tc.pkg)
		filename, src := tc.scaffold(sub, pkgDir)
		gotest.NoError(sub, os.WriteFile(filepath.Join(pkgDir, filename), src, 0o600))
		vet := exec.Command("go", "vet", ".")
		vet.Dir = pkgDir
		out, err := vet.CombinedOutput()
		gotest.NoError(sub, err, "go vet:\n%s\n--- %s ---\n%s", out, filename, src)
	}
}

func (s *ScaffoldCompilesTestSuite) TestFileSuiteNames(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		file string
		want string
	}{
		{"snake case", "user_service", "UserService"},
		{"a dash", "user-service", "UserService"},
		{"a dot", "user.service", "UserService"},
		{"a leading digit", "2fa", "File2fa"},
	}) {
		gotest.Equal(sub, tc.want, scaffold.ExportToPascalCase(tc.file))
	}
}
