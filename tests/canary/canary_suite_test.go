// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this file. The canary anchors
// verdicts outside gotest's reporting stack: it runs the built CLI over
// fixture packages and checks exit codes, the raw go test -json stream and
// marker files against a checked-in golden list, with plain Go only.
package canary_test //nolint:fail-guard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CanaryTestSuite runs one fixture package per method. Sequential: it builds
// the CLI once and each method spawns a full pipeline run.
type CanaryTestSuite struct {
	binary   string
	expected map[string]expectation
}

type expectation struct {
	exit int
	rows []string
}

func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

func (s *CanaryTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	name := "gotest"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	s.binary = filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fatalf(t, "build gotest: %v\n%s", err, out)
	}
	s.expected = readExpected(t, filepath.Join("testdata", "expected.txt"))
}

func (s *CanaryTestSuite) AfterAll(t *gotest.T) {}

// readExpected parses the golden list: a "<pkg> exit <code>" line opens a
// package, and "<pkg> <action> <test>" lines follow it.
func readExpected(t *gotest.T, path string) map[string]expectation {
	f, err := os.Open(path)
	if err != nil {
		fatalf(t, "open golden: %v", err)
	}
	defer f.Close()
	out := map[string]expectation{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 {
			continue
		}
		pkg := fields[0]
		e := out[pkg]
		if fields[1] == "exit" {
			code, err := strconv.Atoi(fields[2])
			if err != nil {
				fatalf(t, "golden exit code for %s: %v", pkg, err)
			}
			e.exit = code
		} else {
			e.rows = append(e.rows, strings.Join(fields, " "))
		}
		out[pkg] = e
	}
	if err := sc.Err(); err != nil {
		fatalf(t, "read golden: %v", err)
	}
	return out
}

type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

// run executes the CLI in -json mode over one fixture package and returns
// its exit code and the verdict rows it produced, in golden form.
func (s *CanaryTestSuite) run(t *gotest.T, pkg, canaryDir string) (int, []string) {
	cmd := exec.Command(s.binary, "-json", "./testdata/"+pkg+"/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = append(os.Environ(), "GOTEST_CANARY_DIR="+canaryDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fatalf(t, "run %s: %v", pkg, err)
		}
		code = exitErr.ExitCode()
	}
	seen := map[string]bool{}
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Action != "pass" && ev.Action != "fail" && ev.Action != "skip" {
			continue
		}
		test := ev.Test
		if test == "" {
			test = "-"
		}
		seen[pkg+" "+ev.Action+" "+test] = true
	}
	rows := make([]string, 0, len(seen))
	for r := range seen {
		rows = append(rows, r)
	}
	sort.Strings(rows)
	return code, rows
}

// check runs a fixture and compares exit code and rows with the golden.
func (s *CanaryTestSuite) check(t *gotest.T, pkg string) string {
	want, ok := s.expected[pkg]
	if !ok {
		fatalf(t, "no golden entry for %s", pkg)
	}
	dir := t.TempDir()
	code, rows := s.run(t, pkg, dir)
	if code != want.exit {
		t.Errorf("%s: exit code %d, want %d", pkg, code, want.exit)
	}
	if strings.Join(rows, "\n") != strings.Join(want.rows, "\n") {
		t.Errorf("%s: verdicts differ from the golden list\n got:\n  %s\n want:\n  %s",
			pkg, strings.Join(rows, "\n  "), strings.Join(want.rows, "\n  "))
	}
	return dir
}

func (s *CanaryTestSuite) TestPassingSuiteIsGreen(t *gotest.T) {
	s.check(t, "passing")
}

func (s *CanaryTestSuite) TestEveryAssertionFamilyFails(t *gotest.T) {
	s.check(t, "failing")
}

func (s *CanaryTestSuite) TestAnAssertionHaltsTheMethod(t *gotest.T) {
	dir := s.check(t, "failnow")
	if _, err := os.Stat(filepath.Join(dir, "after-failnow")); err == nil {
		t.Errorf("the statement after a failed assertion ran: the assertion did not halt")
	}
}

func (s *CanaryTestSuite) TestAfterEachRunsWhenTheTestFailed(t *gotest.T) {
	dir := s.check(t, "aftereach")
	if _, err := os.Stat(filepath.Join(dir, "aftereach-ran")); err != nil {
		t.Errorf("AfterEach did not run after a failing test: %v", err)
	}
}

func (s *CanaryTestSuite) TestLifecycleRunsInOrder(t *gotest.T) {
	dir := s.check(t, "lifecycle")
	got, err := os.ReadFile(filepath.Join(dir, "lifecycle"))
	if err != nil {
		fatalf(t, "lifecycle record: %v", err)
	}
	if want := "beforeAll\nTestFirst\nTestSecond\nafterAll\n"; string(got) != want {
		t.Errorf("lifecycle order:\n got  %q\n want %q", got, want)
	}
}

func (s *CanaryTestSuite) TestAnUncompilablePackageIsExit2(t *gotest.T) {
	s.check(t, "broken")
}

// A panic in a method fails the method and ends the test binary, so a
// sibling declared after it gets no verdict; the run is red regardless.
func (s *CanaryTestSuite) TestAPanicFailsTheMethod(t *gotest.T) {
	s.check(t, "panicking")
}
