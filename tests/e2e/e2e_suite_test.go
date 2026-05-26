package e2e_test

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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
	binaryName := "gotest"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	s.binary = filepath.Join(binDir, binaryName)
	cmd := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest")
	cmd.Dir = absRoot
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "build gotest binary: %s", string(out))

	s.workDir = t.T().TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), s.workDir, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), s.workDir)
	testutils.HackGoWork(t.T(), s.workDir)
}

func (s *E2ETestSuite) AfterAll(t *gotest.T) {}

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
		"pkg/gotest/gotest_",
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

func (s *E2ETestSuite) TestOutputFormat(t *gotest.T) {
	pkgSummaryRe := regexp.MustCompile(`^(ok|FAIL)\s+\S+\s+\d+\.\d+s$`)

	t.When("non-verbose mode", func(w *gotest.T) {
		w.It("produces only summary lines without PASS prefix", func(it *gotest.T) {
			cmd := exec.Command(s.binary, filepath.Join(s.workDir, "examples", "auth"))
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()
			output := strings.TrimSpace(string(out))

			gotest.NoError(it, err, "auth suite should pass: %s", output)

			lines := strings.Split(output, "\n")
			for _, line := range lines {
				gotest.True(it, pkgSummaryRe.MatchString(line),
					"every line should be a package summary, got: %q", line)
			}
			gotest.False(it, strings.Contains(output, "PASS"),
				"non-verbose output should not contain PASS")
			gotest.False(it, strings.Contains(output, "=== RUN"),
				"non-verbose output should not contain verbose test output")
		})

		w.It("produces summary lines for multiple packages", func(it *gotest.T) {
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth", "github.com/mvrahden/go-test/examples/cart")
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()
			output := strings.TrimSpace(string(out))

			gotest.NoError(it, err, "suites should pass: %s", output)

			lines := strings.Split(output, "\n")
			gotest.Equal(it, 2, len(lines), "expected two summary lines, got: %q", output)
			for _, line := range lines {
				gotest.True(it, pkgSummaryRe.MatchString(line),
					"each line should be a package summary, got: %q", line)
			}
		})
	})

	t.When("verbose mode", func(w *gotest.T) {
		w.It("includes PASS prefix before ok summary", func(it *gotest.T) {
			cmd := exec.Command(s.binary, filepath.Join(s.workDir, "examples", "auth"), "-v")
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()
			output := string(out)

			gotest.NoError(it, err, "auth suite should pass: %s", output)
			gotest.Contains(it, output, "=== RUN")
			gotest.Contains(it, output, "--- PASS:")
			gotest.Contains(it, output, "PASS\nok  \t")
		})
	})

	t.When("failing tests", func(w *gotest.T) {
		w.It("shows failure output even without -v", func(it *gotest.T) {
			failDir := filepath.Join(s.workDir, "examples", "fail_fmt")
			os.MkdirAll(failDir, 0o755)
			defer os.RemoveAll(failDir)
			os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte(
				"package failfmt\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailFmtTestSuite struct{}\n\nfunc (s *FailFmtTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n",
			), 0o644)

			cmd := exec.Command(s.binary, failDir)
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, _ := cmd.CombinedOutput()
			output := string(out)

			gotest.Contains(it, output, "FAIL")
			gotest.Contains(it, output, "--- FAIL:")
		})

		w.It("emits trailing FAIL after all packages when any fails", func(it *gotest.T) {
			failDir := filepath.Join(s.workDir, "examples", "fail_trail")
			os.MkdirAll(failDir, 0o755)
			defer os.RemoveAll(failDir)
			os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte(
				"package failtrail\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailTrailTestSuite struct{}\n\nfunc (s *FailTrailTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n",
			), 0o644)

			cmd := exec.Command(s.binary, filepath.Join(s.workDir, "examples", "auth"), failDir)
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, _ := cmd.CombinedOutput()
			output := string(out)

			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			gotest.Equal(it, "FAIL", lines[len(lines)-1],
				"last line should be trailing FAIL, got output: %q", output)
		})
	})

	t.When("package ordering", func(w *gotest.T) {
		w.It("outputs packages in argument order for multiple packages", func(it *gotest.T) {
			cmd := exec.Command(s.binary,
				"github.com/mvrahden/go-test/examples/cart",
				"github.com/mvrahden/go-test/examples/auth",
			)
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()
			output := strings.TrimSpace(string(out))

			gotest.NoError(it, err, "suites should pass: %s", output)

			lines := strings.Split(output, "\n")
			gotest.True(it, len(lines) >= 2, "expected at least 2 lines, got: %q", output)
			gotest.Contains(it, lines[0], "examples/cart",
				"first line should be cart (listed first), got: %q", lines[0])
			gotest.Contains(it, lines[1], "examples/auth",
				"second line should be auth (listed second), got: %q", lines[1])
		})
	})

	t.When("cached results", func(w *gotest.T) {
		w.It("shows (cached) on second run", func(it *gotest.T) {
			it.T().Skip("cache support not yet implemented")
		})
	})
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

func (s *E2ETestSuite) TestOutputFormatGolden(t *gotest.T) {
	t.When("non-verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth")
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "auth should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeOutput(string(out), s.workDir))
		})

		w.It("multi-package mixed with failure", func(it *gotest.T) {
			failDir := filepath.Join(s.workDir, "examples", "fail_golden")
			gotest.NoError(it, os.MkdirAll(failDir, 0o755))
			defer os.RemoveAll(failDir)
			gotest.NoError(it, os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte(
				"package failgolden\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailGoldenTestSuite struct{}\n\nfunc (s *FailGoldenTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n",
			), 0o644))

			cmd := exec.Command(s.binary,
				"github.com/mvrahden/go-test/examples/fail_golden",
				"github.com/mvrahden/go-test/examples/auth",
			)
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, _ := cmd.CombinedOutput()

			gotest.MatchSnapshot(it, normalizeOutput(string(out), s.workDir))
		})
	})

	t.When("json", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth", "-json", "-parallel", "1")
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "auth should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeJSONOutput(string(out)))
		})
	})

	t.When("verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth", "-v", "-parallel", "1")
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "auth should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeOutput(string(out), s.workDir))
		})
	})
}

func normalizeOutput(raw string, workDir string) string {
	s := strings.ReplaceAll(raw, workDir, "<REPLACED>")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	re := regexp.MustCompile(`\d+\.\d+s`)
	return re.ReplaceAllString(s, "<TIMESTAMP>")
}

func normalizeJSONOutput(raw string) string {
	re := regexp.MustCompile(`\d+\.\d+s`)
	var lines []string
	for line := range strings.SplitSeq(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			lines = append(lines, line)
			continue
		}
		ev["Time"] = "<TIMESTAMP>"
		if _, ok := ev["Elapsed"]; ok {
			ev["Elapsed"] = "<TIMESTAMP>"
		}
		if output, ok := ev["Output"].(string); ok {
			ev["Output"] = re.ReplaceAllString(output, "<TIMESTAMP>")
		}
		normalized, _ := json.Marshal(ev)
		lines = append(lines, string(normalized))
	}
	return strings.Join(lines, "\n") + "\n"
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
