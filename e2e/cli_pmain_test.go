package e2e

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
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
		".git",                    // entire .git dir
		"go.work",                 // no go.work reference
		"pkg/gotest/main_test.go", // this file
	}
	testutils.CopyModuleUnderTestToTmp(t, tmp, "./..", excludedPaths...)
	testutils.ActivateTests(t, tmp)

	unexpectedFiles := []string{
		"go.work",
		"examples/gosuite/ƒƒ_psuite_test.go",
		"examples/gosuite/ƒƒ_pxsuite_test.go",
	}
	testutils.AssertFilesNotInTmp(t, tmp, unexpectedFiles...)
	// assert package to test is in tmp
	expectedFiles := []string{
		"go.mod",
		"examples/gosuite/prog_suite_test.go",
	}
	testutils.AssertFilesInTmp(t, tmp, expectedFiles...)
	testutils.HackGoWork(t, tmp)

	testCases := []struct {
		pkgName    string
		args       []string
		goldenName string
		debug      bool
	}{
		{pkgName: "examples/gosuite", goldenName: "gosuite_output.txt", args: []string{"-dir", ""}, debug: false},
		{pkgName: "examples/my", goldenName: "my_output.txt", args: []string{"-dir", ""}, debug: false},
		{pkgName: "examples/simple_suite", goldenName: "simple_suite_output.txt", args: []string{"-dir", ""}, debug: false},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("idx %d", idx), func(t *testing.T) {
			performTest(t, tmp, tc.pkgName, tc.goldenName, tc.debug)
		})
	}
}

func performTest(t *testing.T, tmpDir, pkgPath, goldenName string, debug bool) {
	tmpCurrentPackage := filepath.Join(tmpDir, pkgPath)
	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/testsuite", tmpCurrentPackage)
	if debug {
		cmd.Args = append(cmd.Args, "-internal.debug")
		exec.Command("sh", "-c", `echo "`+tmpCurrentPackage+`" >> debug_dirs`).CombinedOutput()
	}
	cmd.Args = append(cmd.Args, "-", "-v")
	cmd.Dir = tmpDir
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
	for _, suiteName := range []string{about.PSuite, about.PXSuite} {
		file, err := os.Stat(filepath.Join(tmpDir, pkgPath, suiteName))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return
			}
			t.Errorf("failed asserting file: %s", file.Name())
		}
		if file.Size() > 0 {
			t.Errorf("failed asserting file: %s", file.Name())
		}
	}
}
