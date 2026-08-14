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
	"time"

	"github.com/mvrahden/go-test/internal/about"
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

// SuiteConfig: Exclusive — every test method here shells the built CLI,
// which compiles packages (often with -race) per invocation. That load must
// never run beside the timing-budget harnesses. Keep invocations frugal
// too: one CLI run per distinct pipeline behavior, assertions merged into
// it, binaries and module copies built once in BeforeAll, workloads pinned
// tiny (-benchtime=10x scale).
func (s *E2ETestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

func (s *E2ETestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)

	binDir := t.TempDir()
	binaryName := "gotest"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	s.binary = filepath.Join(binDir, binaryName)
	cmd := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	cmd.Dir = absRoot
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "build gotest binary: %s", string(out))

	s.workDir = t.TempDir()
	testutils.CopyModuleUnderTestToTmp(t.T(), s.workDir, "../..", testutils.DefaultExcludePaths...)
	testutils.ActivateTests(t.T(), s.workDir)
	testutils.HackGoWork(t.T(), s.workDir)
}

func (s *E2ETestSuite) AfterAll(t *gotest.T) {}

func (s *E2ETestSuite) TestT(t *gotest.T) {
	tmp := t.TempDir()
	excludedPaths := append(append([]string(nil), testutils.DefaultExcludePaths...),
		"pkg/gotest/assertions_suite_test.go",
		"pkg/gotest/b_suite_test.go",
		"pkg/gotest/config_suite_test.go",
		"pkg/gotest/each_filter_suite_test.go",
		"pkg/gotest/each_suite_test.go",
		"pkg/gotest/export_test.go",
		"pkg/gotest/must_suite_test.go",
		"pkg/gotest/record_suite_test.go",
		"pkg/gotest/snapshot_internal_test.go",
		"pkg/gotest/snapshot_suite_test.go",
		"pkg/gotest/t_suite_test.go",
		"pkg/gotest/linereport_helpers_test.go",
		"pkg/gotest/linereport_suite_test.go",
		"pkg/gotest/gotest_",
	)
	testutils.CopyModuleUnderTestToTmp(t.T(), tmp, "../..", excludedPaths...)
	placeFixture(t.T(), tmp, "t_test.go", "pkg/gotest/t_test.go")
	testutils.AssertFilesNotInTmp(t.T(), tmp, "go.work")
	testutils.AssertFilesInTmp(t.T(), tmp, "go.mod", "pkg/gotest/t_test.go", "pkg/gotest/t.go")
	testutils.HackGoWork(t.T(), tmp)

	cmd := exec.Command(s.binary, filepath.Join(tmp, "pkg/gotest"), "-v") //nolint:gosec // G204: controlled binary with fixed args
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
	cmd := exec.Command(s.binary, filepath.Join(s.workDir, "examples", "search"), "-v") //nolint:gosec // G204: controlled binary with fixed args
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
	cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/...", "-v") //nolint:gosec // G204: controlled binary with fixed args
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
	_ = os.MkdirAll(failDir, 0o755)
	defer os.RemoveAll(failDir)
	_ = os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte("package failsuite\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailTestSuite struct{}\n\nfunc (s *FailTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n"), 0o600)

	cmd := exec.Command(s.binary, failDir, "-v") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = filepath.Join(s.workDir, "examples")
	_, err := cmd.CombinedOutput()

	gotest.Error(t, err, "expected non-zero exit code for failing tests")
	exitErr, ok := err.(*exec.ExitError)
	gotest.True(t, ok, "expected *exec.ExitError, got %T: %v", err, err)
	gotest.NotEqual(t, exitErr.ExitCode(), 0, "expected non-zero exit code")
}

func (s *E2ETestSuite) TestSharedFixtureExitTiming(t *gotest.T) {
	t.When("running packages with shared fixtures", func(w *gotest.T) {
		w.It("exits promptly after all tests complete", func(it *gotest.T) {
			cmd := exec.Command(s.binary, //nolint:gosec // G204: controlled binary with fixed args
				"github.com/mvrahden/go-test/tests/sharedfixture/...",
				"-json", "-count=1")
			cmd.Dir = s.workDir

			start := time.Now()
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(start)

			gotest.NoError(it, err, "shared fixture tests should pass: %s", string(out))
			gotest.Less(it, elapsed, 60*time.Second, "should exit promptly after tests complete (no process hang), took %v", elapsed)
		})
	})
}

func (s *E2ETestSuite) TestOutputFormatGolden(t *gotest.T) {
	t.When("non-verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth") //nolint:gosec // G204: controlled binary with fixed args
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "auth should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeOutput(string(out), s.workDir))
		})

		w.It("multi-package all passing", func(it *gotest.T) {
			cmd := exec.Command(s.binary, //nolint:gosec // G204: controlled binary with fixed args
				"github.com/mvrahden/go-test/examples/cart",
				"github.com/mvrahden/go-test/examples/auth",
			)
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "both packages should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeOutput(string(out), s.workDir))
		})

		w.It("multi-package mixed with failure", func(it *gotest.T) {
			failDir := filepath.Join(s.workDir, "examples", "fail_golden")
			gotest.NoError(it, os.MkdirAll(failDir, 0o755))
			defer os.RemoveAll(failDir)
			gotest.NoError(it, os.WriteFile(filepath.Join(failDir, "ptest_test.go"), []byte(
				"package failgolden\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\ntype FailGoldenTestSuite struct{}\n\nfunc (s *FailGoldenTestSuite) TestAlwaysFails(t *gotest.T) { t.FailNow() }\n",
			), 0o600))

			cmd := exec.Command(s.binary, //nolint:gosec // G204: controlled binary with fixed args
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
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth", "-json", "-parallel", "1") //nolint:gosec // G204: controlled binary with fixed args
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "auth should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeJSONOutput(string(out)))
		})
	})

	t.When("verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			cmd := exec.Command(s.binary, "github.com/mvrahden/go-test/examples/auth", "-v", "-parallel", "1") //nolint:gosec // G204: controlled binary with fixed args
			cmd.Dir = filepath.Join(s.workDir, "examples")
			out, err := cmd.CombinedOutput()

			gotest.NoError(it, err, "auth should pass: %s", string(out))
			gotest.MatchSnapshot(it, normalizeOutput(string(out), s.workDir))
		})
	})
}

func (s *E2ETestSuite) TestBenchJSONReport(t *gotest.T) {
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")

	runBench := func(it *gotest.T, extra ...string) []byte {
		args := append([]string{"bench", "github.com/mvrahden/go-test/examples/benchmarking",
			"-bench=^BenchmarkCacheTestSuite$/^BenchmarkGetHit$", "-benchtime=10x"}, extra...)
		cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = filepath.Join(s.workDir, "examples")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		gotest.NoError(it, cmd.Run(), "bench run failed:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
		return stdout.Bytes()
	}

	type reportDoc struct {
		SchemaVersion int `json:"schemaVersion"`
		Baseline      struct {
			SchemaVersion int    `json:"schemaVersion"`
			GOOS          string `json:"goos"`
			Results       []struct {
				Suite   string `json:"suite"`
				Name    string `json:"name"`
				Samples []struct {
					Iterations int     `json:"iterations"`
					NsPerOp    float64 `json:"nsPerOp"`
				} `json:"samples"`
			} `json:"results"`
		} `json:"baseline"`
		Deltas []struct {
			Key         string `json:"key"`
			Significant bool   `json:"significant"`
		} `json:"deltas"`
		Gate *struct {
			ThresholdPct float64 `json:"thresholdPct"`
			Breached     bool    `json:"breached"`
		} `json:"gate"`
	}

	// Two CLI invocations total, not three: the save run doubles as the
	// plain-report probe. Every bench invocation compiles the example with
	// -race, and this suite runs concurrently with the timing-sensitive
	// budget harnesses — the gate has no headroom for redundant load.
	t.When("--json runs a slash-scoped single method and saves a baseline", func(w *gotest.T) {
		out := runBench(w, "--save="+baselinePath, "--json")
		var report reportDoc
		gotest.NoError(w, json.Unmarshal(out, &report), "stdout must be one JSON document:\n%s", out)

		w.It("emits the versioned report with exactly the scoped method", func(it *gotest.T) {
			gotest.Equal(it, 1, report.SchemaVersion)
			gotest.Equal(it, 1, report.Baseline.SchemaVersion)
			gotest.Len(it, report.Baseline.Results, 1)
			gotest.Equal(it, "CacheTestSuite", report.Baseline.Results[0].Suite)
			gotest.Equal(it, "BenchmarkGetHit", report.Baseline.Results[0].Name)
			gotest.Equal(it, 10, report.Baseline.Results[0].Samples[0].Iterations)
		})

		w.It("omits deltas and gate when no comparison ran", func(it *gotest.T) {
			gotest.Empty(it, report.Deltas)
			gotest.Zero(it, report.Gate)
		})
	})

	t.When("--json compares against the saved baseline with a gate", func(w *gotest.T) {
		out := runBench(w, "--against="+baselinePath, "--gate=1000", "--json")
		var report reportDoc
		gotest.NoError(w, json.Unmarshal(out, &report), "stdout must be one JSON document:\n%s", out)

		w.It("carries one delta per matched benchmark and the gate verdict", func(it *gotest.T) {
			gotest.Len(it, report.Deltas, 1)
			gotest.Contains(it, report.Deltas[0].Key, "CacheTestSuite/BenchmarkGetHit")
			gotest.NotZero(it, report.Gate)
			gotest.Equal(it, 1000.0, report.Gate.ThresholdPct)
			gotest.False(it, report.Gate.Breached)
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
	gotest.NoError(t, err, "open fixture %s: %v", srcName, err)
	defer src.Close()
	dst := filepath.Join(tmpDir, dstRel)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	f, err := os.Create(dst)
	gotest.NoError(t, err, "create %s: %v", dst, err)
	defer f.Close()
	_, err = io.Copy(f, src)
	gotest.NoError(t, err, "copy fixture")
}

func (s *E2ETestSuite) performTest(t *testing.T, basedir, pkgPath, pkgName, goldenName string) {
	t.Helper()
	unifiedPkgDescriptor := pkgName
	if unifiedPkgDescriptor == "" {
		unifiedPkgDescriptor = filepath.Join(s.workDir, basedir, pkgPath)
	}

	cmd := exec.Command(s.binary, unifiedPkgDescriptor, "-v", "-parallel", "1") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = filepath.Join(s.workDir, basedir)
	out, _ := cmd.CombinedOutput()

	testutils.CompareTestOutputWithGolden(t, s.workDir, bytes.NewBuffer(out), testdataFS, goldenName)

	_ = fs.WalkDir(os.DirFS(s.workDir), basedir, func(path string, d fs.DirEntry, err error) error {
		gotest.False(t, about.PSuiteRegex.MatchString(path), "found test suite after execution")
		return nil
	})
}
