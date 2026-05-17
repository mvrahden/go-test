package e2e

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/tests/e2e/internal/testutils"
	"github.com/mvrahden/go-test/pkg/gotest"
)

//go:embed testdata
var testdataFS embed.FS

// Test_TestsuiteCLI tests the full CLI execution as a blackbox and
// makes golden assertions against the CLI output.
func Test_TestsuiteCLI(t *testing.T) {
	// Create test directory with test files
	tmp := t.TempDir()

	// clone module into tmp
	testutils.CopyModuleUnderTestToTmp(t, tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t, tmp)

	unexpectedFiles := []string{
		"go.work",
		"examples/auth/ƒƒ_psuite_test.go",
		"examples/auth/ƒƒ_pxsuite_test.go",
	}
	testutils.AssertFilesNotInTmp(t, tmp, unexpectedFiles...)
	// assert package to test is in tmp
	expectedFiles := []string{
		"go.mod",
		"examples/auth/validator.go",
		"examples/auth/suite_test.go",
	}
	testutils.AssertFilesInTmp(t, tmp, expectedFiles...)
	testutils.HackGoWork(t, tmp)

	testCases := []struct {
		desc       string
		basedir    string
		pkgName    string
		pkgPath    string
		goldenName string
		debug      bool
	}{
		{desc: "auth by relative path", basedir: "examples", pkgPath: "auth", goldenName: "auth_output.txt"},
		{desc: "cart by relative path", basedir: "examples", pkgPath: "cart", goldenName: "cart_output.txt"},
		{desc: "auth by package name", basedir: "examples", pkgName: "github.com/mvrahden/go-test/examples/auth", goldenName: "auth_output.txt"},
		{desc: "cart by package name", basedir: "examples", pkgName: "github.com/mvrahden/go-test/examples/cart", goldenName: "cart_output.txt"},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			performTest(t, tmp, tc.basedir, tc.pkgPath, tc.pkgName, tc.goldenName, tc.debug)
		})
	}
}

func Test_TestsuiteCLI_ParallelSuite(t *testing.T) {
	tmp := t.TempDir()
	testutils.CopyModuleUnderTestToTmp(t, tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t, tmp)
	testutils.HackGoWork(t, tmp)

	cmd := exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest",
		filepath.Join(tmp, "examples", "search"), "-v")
	cmd.Dir = filepath.Join(tmp, "examples")
	out, err := cmd.CombinedOutput()
	output := string(out)

	gotest.NoError(t, err, "parallel suite should pass: %s", output)
	gotest.Contains(t, output, "TestArticleSearchTestSuite")
	gotest.Contains(t, output, "TestSearchByTitle")
	gotest.Contains(t, output, "TestSearchByBody")
	gotest.Contains(t, output, "TestArticleIndexTestSuite")
	gotest.Contains(t, output, "TestProductIndexTestSuite")
	gotest.Contains(t, output, "TestSearchResultTestSuite")
	gotest.Contains(t, output, "PAUSE")
	gotest.Contains(t, output, "PASS")
}

func Test_TestsuiteCLI_GenericSuite(t *testing.T) {
	tmp := t.TempDir()
	testutils.CopyModuleUnderTestToTmp(t, tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t, tmp)
	testutils.HackGoWork(t, tmp)

	cmd := exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest",
		filepath.Join(tmp, "examples", "search"), "-v")
	cmd.Dir = filepath.Join(tmp, "examples")
	out, err := cmd.CombinedOutput()
	output := string(out)

	gotest.NoError(t, err, "generic suite should pass: %s", output)
	gotest.Contains(t, output, "TestArticleIndexTestSuite")
	gotest.Contains(t, output, "TestProductIndexTestSuite")
	gotest.Contains(t, output, "TestEmptyIndex")
	gotest.Contains(t, output, "PASS")
}

func Test_TestsuiteCLI_AllPackages(t *testing.T) {
	tmp := t.TempDir()
	testutils.CopyModuleUnderTestToTmp(t, tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t, tmp)
	testutils.HackGoWork(t, tmp)

	cmd := exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest",
		"github.com/mvrahden/go-test/examples/...", "-v")
	cmd.Dir = filepath.Join(tmp, "examples")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	gotest.Contains(t, output, "TestTokenValidatorTestSuite")
	gotest.Contains(t, output, "TestShoppingCartTestSuite")
	gotest.Contains(t, output, "TestArticleSearchTestSuite")
	gotest.Contains(t, output, "TestNotificationServiceTestSuite")
}

func Test_TestsuiteCLI_ExitCode(t *testing.T) {
	tmp := t.TempDir()
	testutils.CopyModuleUnderTestToTmp(t, tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t, tmp)
	testutils.HackGoWork(t, tmp)

	failDir := filepath.Join(tmp, "examples", "fail_suite")
	os.MkdirAll(failDir, 0o755)
	os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte("package failsuite\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailTestSuite struct{}\n\nfunc (s *FailTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n"), 0o644)

	cmd := exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest",
		failDir, "-v")
	cmd.Dir = filepath.Join(tmp, "examples")
	_, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("expected non-zero exit code for failing tests")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func performTest(t *testing.T, tmpDir, basedir, inPkgPath, inPkgName, goldenName string, debug bool) {
	unifiedPkgDesciptor := inPkgName // either pkgName or build absolute path
	if unifiedPkgDesciptor == "" {
		unifiedPkgDesciptor = filepath.Join(tmpDir, basedir, inPkgPath)
	}

	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest", unifiedPkgDesciptor)
	if debug {
		cmd.Args = append(cmd.Args, "-ƒƒ.internal.debug")
		exec.Command("sh", "-c", `echo "`+unifiedPkgDesciptor+`" >> debug_dirs`).CombinedOutput()
	}
	cmd.Args = append(cmd.Args, "-v", "-parallel", "1")
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
