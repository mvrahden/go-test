package e2e_test

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/e2e/internal/testutils"
)

//go:embed testdata
var testdataFS embed.FS

type E2ETestSuite struct{}

func (s *E2ETestSuite) TestT(t *gotest.T) {
	tmp := t.T().TempDir()

	// clone module into tmp — exclude all real test files from pkg/gotest
	excludedPaths := append(testutils.DefaultExcludePaths,
		"pkg/gotest/assertions_suite_test.go",
		"pkg/gotest/config_suite_test.go",
		"pkg/gotest/each_suite_test.go",
		"pkg/gotest/export_test.go",
		"pkg/gotest/must_suite_test.go",
		"pkg/gotest/record_suite_test.go",
		"pkg/gotest/snapshot_internal_test.go",
		"pkg/gotest/snapshot_suite_test.go",
		"pkg/gotest/t_suite_test.go",
		"pkg/gotest/ƒƒ_",
	)
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", excludedPaths...)
	placeFixture(t.T(), tmp, "t_test.go", "pkg/gotest/t_test.go")

	testutils.AssertFilesNotInTmp(t.T(), tmp, "go.work")
	testutils.AssertFilesInTmp(t.T(), tmp, "go.mod", "pkg/gotest/t_test.go", "pkg/gotest/t.go")
	testutils.HackGoWork(t.T(), tmp)

	tmpCurrentPackage := filepath.Join(tmp, "/pkg/gotest")
	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest", tmpCurrentPackage, "-v")
	cmd.Dir = tmp
	out, _ := cmd.CombinedOutput()

	testutils.CompareTestOutputWithGolden(
		t.T(),
		tmp,
		bytes.NewBuffer(out),
		testdataFS,
		"t.golden",
	)
}

func (s *E2ETestSuite) TestTestsuiteCLI(t *gotest.T) {
	// Create test directory with test files
	tmp := t.T().TempDir()

	// clone module into tmp
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), tmp)

	unexpectedFiles := []string{
		"go.work",
		"examples/auth/ƒƒ_psuite_test.go",
		"examples/auth/ƒƒ_pxsuite_test.go",
	}
	testutils.AssertFilesNotInTmp(t.T(), tmp, unexpectedFiles...)
	// assert package to test is in tmp
	expectedFiles := []string{
		"go.mod",
		"examples/auth/validator.go",
		"examples/auth/suite_test.go",
	}
	testutils.AssertFilesInTmp(t.T(), tmp, expectedFiles...)
	testutils.HackGoWork(t.T(), tmp)

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
		t.It(tc.desc, func(it *gotest.T) {
			performTest(it.T(), tmp, tc.basedir, tc.pkgPath, tc.pkgName, tc.goldenName, tc.debug)
		})
	}
}

func (s *E2ETestSuite) TestTestsuiteCLIParallelSuite(t *gotest.T) {
	tmp := t.T().TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), tmp)
	testutils.HackGoWork(t.T(), tmp)

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

func (s *E2ETestSuite) TestTestsuiteCLIGenericSuite(t *gotest.T) {
	tmp := t.T().TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), tmp)
	testutils.HackGoWork(t.T(), tmp)

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

func (s *E2ETestSuite) TestTestsuiteCLIAllPackages(t *gotest.T) {
	tmp := t.T().TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), tmp)
	testutils.HackGoWork(t.T(), tmp)

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

func (s *E2ETestSuite) TestTestsuiteCLIExitCode(t *gotest.T) {
	tmp := t.T().TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), tmp)
	testutils.HackGoWork(t.T(), tmp)

	failDir := filepath.Join(tmp, "examples", "fail_suite")
	os.MkdirAll(failDir, 0o755)
	os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte("package failsuite\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailTestSuite struct{}\n\nfunc (s *FailTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n"), 0o644)

	cmd := exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest",
		failDir, "-v")
	cmd.Dir = filepath.Join(tmp, "examples")
	_, err := cmd.CombinedOutput()

	if err == nil {
		t.T().Fatal("expected non-zero exit code for failing tests")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.T().Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.T().Fatal("expected non-zero exit code")
	}
}

func placeFixture(t *testing.T, tmpDir, srcName, dstRel string) {
	t.Helper()
	src, err := testdataFS.Open("testdata/" + srcName)
	if err != nil {
		t.Fatalf("open fixture %s: %v", srcName, err)
	}
	defer src.Close()
	dst := filepath.Join(tmpDir, dstRel)
	os.MkdirAll(filepath.Dir(dst), 0o755)
	f, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, src); err != nil {
		t.Fatalf("copy fixture: %v", err)
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
