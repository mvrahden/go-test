package e2e

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/testutils"
)

//go:embed testdata
var testdataFS embed.FS

// Test_TestsuiteCLI tests the full CLI execution as a blackbox and
// makes golden assertions against the CLI output.
func Test_TestsuiteCLI(t *testing.T) {
	// Create test directory with test files
	tmp := t.TempDir()

	// clone module into tmp
	excludedPaths := []string{
		".git",    // entire .git dir
		"go.work", // no go.work reference
	}
	testutils.CopyModuleUnderTestToTmp(t, tmp, "./..", excludedPaths...)
	testutils.ActivateTests(t, tmp)

	unexpectedFiles := []string{
		"go.work",
		"examples/stdlib/ƒƒ_psuite_test.go",
		"examples/stdlib/ƒƒ_pxsuite_test.go",
	}
	testutils.AssertFilesNotInTmp(t, tmp, unexpectedFiles...)
	// assert package to test is in tmp
	expectedFiles := []string{
		"go.mod",
		"examples/stdlib/unit.go",
		"examples/stdlib/unit_suite_ptest_test.go",
	}
	testutils.AssertFilesInTmp(t, tmp, expectedFiles...)
	testutils.HackGoWork(t, tmp)

	testCases := []struct {
		basedir    string
		pkgName    string
		pkgPath    string
		goldenName string
		debug      bool
	}{
		{basedir: "examples", pkgPath: "stdlib", goldenName: "stdlib_output.txt", debug: false},
		{basedir: "examples", pkgPath: "simple_suite", goldenName: "simple_suite_output.txt", debug: false},
		{basedir: "examples", pkgName: "github.com/mvrahden/go-test/examples/stdlib", goldenName: "stdlib_output.txt", debug: false},
		{basedir: "examples", pkgName: "github.com/mvrahden/go-test/examples/simple_suite", goldenName: "simple_suite_output.txt", debug: false},
		{basedir: "examples", pkgName: "github.com/mvrahden/go-test/examples/...", goldenName: "test_all.txt", debug: false},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("idx %d", idx), func(t *testing.T) {
			performTest(t, tmp, tc.basedir, tc.pkgPath, tc.pkgName, tc.goldenName, tc.debug)
		})
	}
}

func performTest(t *testing.T, tmpDir, basedir, inPkgPath, inPkgName, goldenName string, debug bool) {
	unifiedPkgDesciptor := inPkgName // either pkgName or build absolute path
	if unifiedPkgDesciptor == "" {
		unifiedPkgDesciptor = filepath.Join(tmpDir, basedir, inPkgPath)
	}

	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/testsuite", unifiedPkgDesciptor)
	if debug {
		cmd.Args = append(cmd.Args, "-ƒƒ.internal.debug")
		exec.Command("sh", "-c", `echo "`+unifiedPkgDesciptor+`" >> debug_dirs`).CombinedOutput()
	}
	cmd.Args = append(cmd.Args, "-v")
	cmd.Dir = filepath.Join(tmpDir, basedir)
	out, _ := cmd.CombinedOutput()

	// assert output
	testutils.CompareTestOutputWithGolden(
		t,
		tmpDir,
		bytes.NewBuffer(out),
		testdataFS,
		goldenName,
	)

	// assert testsuite was removed after execution
	fs.WalkDir(os.DirFS(tmpDir), basedir, func(path string, d fs.DirEntry, err error) error {
		if about.PSuiteRegex.MatchString(path) {
			t.Fatalf("found test suite after executions")
		}
		return nil
	})
}
