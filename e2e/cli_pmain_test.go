package gosuite

import (
	"bytes"
	"embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
		"examples/gosuite/ff_suite_test.go",
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
		goldenName string
		debug      bool
	}{
		{pkgName: "examples/gosuite", goldenName: "gosuite_output.txt"},
		{pkgName: "examples/my", goldenName: "my_output.txt"},
		{pkgName: "examples/simple_suite", goldenName: "simple_suite_output.txt"},
	}
	for _, tc := range testCases {
		performTest(t, tmp, tc.pkgName, tc.goldenName, tc.debug)
	}
}

func performTest(t *testing.T, tmpDir, pkgPath, goldenName string, debug bool) {
	tmpCurrentPackage := filepath.Join(tmpDir, pkgPath)
	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/testsuite", "-dir", tmpCurrentPackage)
	if debug {
		cmd.Args = append(cmd.Args, "-internal.debug")
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

	// assert testsuite was removed during execution
	file, err := os.Stat(filepath.Join(tmpDir, pkgPath, "ƒƒ_suite_test.go"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		t.Errorf("failed asserting file: %s", file)
	}
	if file.Size() > 0 {
		t.Errorf("failed asserting file: %s", file)
	}
}
