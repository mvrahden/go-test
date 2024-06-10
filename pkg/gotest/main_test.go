package gotest_test

import (
	"bytes"
	"embed"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/testutils"
)

//go:embed testdata
var testdataFS embed.FS

func TestT(t *testing.T) {
	// In order to test the full round-trip for the
	// `gotest.T` object, this testcase will
	// copy over the project to a tmp dir and
	// initialize the Go module and workspace and then
	// execute the CLI command on it and compare the output
	// with an expected Golden.

	// Create test directory with test files
	tmp := t.TempDir()

	// clone module into tmp
	excludedPaths := []string{
		".git",                    // entire .git dir
		"go.work",                 // no go.work reference
		"pkg/gotest/main_test.go", // this file
	}
	testutils.CopyModuleUnderTestToTmp(t, tmp, "./../..", excludedPaths...)
	testutils.ActivateTests(t, tmp)

	unexpectedFiles := []string{
		"go.work",
		"/pkg/gotest/main_test.go",
	}
	testutils.AssertFilesNotInTmp(t, tmp, unexpectedFiles...)
	// assert package to test is in tmp
	expectedFiles := []string{
		"go.mod",
		"/pkg/gotest/t_test.go",
		"/pkg/gotest/t.go",
	}
	testutils.AssertFilesInTmp(t, tmp, expectedFiles...)
	testutils.HackGoWork(t, tmp)

	// perform testsuite command
	// testsuite command algo:
	// - cleanup generated sources
	// - create suite from source
	// - perform "go test" command
	// - cleanup generated sources
	tmpCurrentPackage := filepath.Join(tmp, "/pkg/gotest")
	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/testsuite", "-dir", tmpCurrentPackage, "-", "-v")
	cmd.Dir = tmp
	out, _ := cmd.CombinedOutput()

	testutils.CompareTestOutputWithGolden(
		t,
		tmp,
		bytes.NewBuffer(out),
		testdataFS,
		"t.golden",
	)
}
