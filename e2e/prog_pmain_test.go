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
	}{
		{pkgName: "examples/gosuite", goldenName: "gosuite_output.txt"},
		{pkgName: "examples/my", goldenName: "my_output.txt"},
	}
	for _, tc := range testCases {
		performTest(t, tmp, tc.pkgName, tc.goldenName)
	}
}

func performTest(t *testing.T, tmpDir, pkgPath, goldenName string) {
	tmpCurrentPackage := filepath.Join(tmpDir, pkgPath)
	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/testsuite", "-dir", tmpCurrentPackage)
	cmd.Dir = tmpDir
	out, _ := cmd.CombinedOutput()

	// Testsuite is removed after execution
	file, err := os.Stat(filepath.Join(tmpDir, pkgPath, "ff_suite_test.go"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Errorf("failed asserting file: %s", file)
	}

	testutils.CompareTestOutputWithGolden(
		t,
		tmpDir,
		bytes.NewBuffer(out),
		testdataFS,
		goldenName,
	)
}
