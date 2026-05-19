package e2e //nolint:stdlib-test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/tests/e2e/internal/testutils"
)

func TestT(t *testing.T) {
	tmp := t.TempDir()

	// clone module into tmp — exclude all real test files from pkg/gotest
	excludedPaths := append(testutils.DefaultExcludePaths,
		"pkg/gotest/assertions_suite_test.go",
		"pkg/gotest/helpers_test.go",
		"pkg/gotest/each_suite_test.go",
		"pkg/gotest/eventually_suite_test.go",
		"pkg/gotest/must_suite_test.go",
		"pkg/gotest/snapshot_suite_test.go",
		"pkg/gotest/collecting_test.go",
		"pkg/gotest/config_suite_test.go",
	)
	testutils.CopyModuleUnderTestToTmp(t, tmp, "../..", excludedPaths...)
	placeFixture(t, tmp, "t_test.go", "pkg/gotest/t_test.go")

	testutils.AssertFilesNotInTmp(t, tmp, "go.work")
	testutils.AssertFilesInTmp(t, tmp, "go.mod", "pkg/gotest/t_test.go", "pkg/gotest/t.go")
	testutils.HackGoWork(t, tmp)

	tmpCurrentPackage := filepath.Join(tmp, "/pkg/gotest")
	cmd := exec.
		Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest", tmpCurrentPackage, "-v")
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
