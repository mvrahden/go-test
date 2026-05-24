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

// E2ETestSuite tests the gotest CLI end-to-end against real packages.
type E2ETestSuite struct {
	binary  string
	workDir string
}

func (s *E2ETestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)

	binDir := t.T().TempDir()
	s.binary = filepath.Join(binDir, "gotest")
	cmd := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest")
	cmd.Dir = absRoot
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "build gotest binary: %s", string(out))

	s.workDir = t.T().TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), s.workDir, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), s.workDir)
	testutils.HackGoWork(t.T(), s.workDir)
}

func (s *E2ETestSuite) TestT(t *gotest.T) {
	tmp := t.T().TempDir()
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

	cmd := exec.Command(s.binary, filepath.Join(tmp, "pkg/gotest"), "-v")
	cmd.Dir = tmp
	out, _ := cmd.CombinedOutput()
	testutils.CompareTestOutputWithGolden(t.T(), tmp, bytes.NewBuffer(out), testdataFS, "t.golden")
}

func (s *E2ETestSuite) TestTestsuiteCLI(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc       string
		basedir    string
		pkgPath    string
		pkgName    string
		goldenName string
	}{
		{Desc: "auth by relative path", basedir: "examples", pkgPath: "auth", goldenName: "auth_output.txt"},
		{Desc: "cart by relative path", basedir: "examples", pkgPath: "cart", goldenName: "cart_output.txt"},
		{Desc: "auth by package name", basedir: "examples", pkgName: "github.com/mvrahden/go-test/examples/auth", goldenName: "auth_output.txt"},
	}) {
		s.performTest(sub.T(), tc.basedir, tc.pkgPath, tc.pkgName, tc.goldenName)
	}
}

func (s *E2ETestSuite) TestTestsuiteCLIParallelSuite(t *gotest.T) {
	cmd := exec.Command(s.binary, filepath.Join(s.workDir, "examples", "search"), "-v")
	cmd.Dir = filepath.Join(s.workDir, "examples")
	out, err := cmd.CombinedOutput()
	output := string(out)

	gotest.NoError(t, err, "parallel suite should pass: %s", output)
	gotest.Contains(t, output, "TestArticleSearchTestSuite")
	gotest.Contains(t, output, "TestSearchByTitle")
	gotest.Contains(t, output, "TestSearchByBody")
	gotest.Contains(t, output, "TestArticleIndexTestSuite")
	gotest.Contains(t, output, "TestProductIndexTestSuite")
	gotest.Contains(t, output, "TestSearchResultTestSuite")
	gotest.Contains(t, output, "TestEmptyIndex")
	gotest.Contains(t, output, "PAUSE")
	gotest.Contains(t, output, "PASS")
}

func (s *E2ETestSuite) TestTestsuiteCLIAllPackages(t *gotest.T) {
	cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/...", "-v")
	cmd.Dir = filepath.Join(s.workDir, "examples")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	gotest.Contains(t, output, "TestTokenValidatorTestSuite")
	gotest.Contains(t, output, "TestShoppingCartTestSuite")
	gotest.Contains(t, output, "TestArticleSearchTestSuite")
	gotest.Contains(t, output, "TestNotificationServiceTestSuite")
}

func (s *E2ETestSuite) TestTestsuiteCLIExitCode(t *gotest.T) {
	failDir := filepath.Join(s.workDir, "examples", "fail_suite")
	os.MkdirAll(failDir, 0o755)
	defer os.RemoveAll(failDir)
	os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte("package failsuite\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailTestSuite struct{}\n\nfunc (s *FailTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n"), 0o644)

	cmd := exec.Command(s.binary, failDir, "-v")
	cmd.Dir = filepath.Join(s.workDir, "examples")
	_, err := cmd.CombinedOutput()

	gotest.True(t, err != nil, "expected non-zero exit code for failing tests")
	exitErr, ok := err.(*exec.ExitError)
	gotest.True(t, ok, "expected *exec.ExitError, got %T: %v", err, err)
	gotest.True(t, exitErr.ExitCode() != 0, "expected non-zero exit code")
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

func (s *E2ETestSuite) performTest(t *testing.T, basedir, pkgPath, pkgName, goldenName string) {
	t.Helper()
	unifiedPkgDescriptor := pkgName
	if unifiedPkgDescriptor == "" {
		unifiedPkgDescriptor = filepath.Join(s.workDir, basedir, pkgPath)
	}

	cmd := exec.Command(s.binary, unifiedPkgDescriptor, "-v", "-parallel", "1")
	cmd.Dir = filepath.Join(s.workDir, basedir)
	out, _ := cmd.CombinedOutput()

	testutils.CompareTestOutputWithGolden(t, s.workDir, bytes.NewBuffer(out), testdataFS, goldenName)

	fs.WalkDir(os.DirFS(s.workDir), basedir, func(path string, d fs.DirEntry, err error) error {
		if about.PSuiteRegex.MatchString(path) {
			t.Fatalf("found test suite after execution")
		}
		return nil
	})
}
