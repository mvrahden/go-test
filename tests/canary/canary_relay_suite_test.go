package canary_test //nolint:fail-guard

import (
	"encoding/json"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CanaryRelayTestSuite checks what the runner makes of a suite's output when
// something goes wrong beside it: a stop the channel cannot carry, a
// converter that dies, a signal from outside, a crash under a method and an
// -exec wrapper that exits before the suite.
// Sequential: each method drives a full pipeline run it started.
type CanaryRelayTestSuite struct{ canary }

func (s *CanaryRelayTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 90 * time.Second, SetupTimeout: 30 * time.Second}
}

func (s *CanaryRelayTestSuite) BeforeAll(t *gotest.T) { s.open(t) }

func (s *CanaryRelayTestSuite) AfterAll(t *gotest.T) {}

// relayPackage is the package-level verdict a -json stream carries for the
// relay fixture: pass, fail, or empty when it carries none.
func relayPackage(stream string) string {
	verdict := ""
	for line := range strings.Lines(stream) {
		var ev struct{ Action, Package, Test string }
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Test == "" && strings.HasSuffix(ev.Package, "/testdata/relay") && (ev.Action == "pass" || ev.Action == "fail") {
			verdict = ev.Action
		}
	}
	return verdict
}

// A suite process that dies under its method fails its package and the run,
// keeps every verdict reported before, a subtest's of that method included,
// and gives the method itself none.
func (s *CanaryRelayTestSuite) TestACrashFailsItsPackage(t *gotest.T) {
	crashes := []string{"exit", "panic"}
	if runtime.GOOS != "windows" {
		crashes = append(crashes, "kill")
	}
	for _, crash := range crashes {
		r := s.runWith(t, "relay", t.TempDir(), []string{"RELAY_CRASH=" + crash}, "-run=^TestCrashTestSuite$")
		out := r.stream + r.stderr
		if r.code != 1 {
			t.Errorf("crash %s: exit code %d, want 1\n%s", crash, r.code, out)
		}
		if v := relayPackage(r.stream); v != "fail" {
			t.Errorf("crash %s: package verdict %q, want fail\n%s", crash, v, out)
		}
		for _, path := range []string{"TestCrashTestSuite/TestBefore/passes", "TestCrashTestSuite/TestBefore", "TestCrashTestSuite/TestCrashes/passes_before_the_end"} {
			if v := streamVerdict(r.stream, path); v != "pass" {
				t.Errorf("crash %s: %s verdict %q, want pass: a verdict reported before the end was lost\n%s", crash, path, v, out)
			}
		}
		for _, path := range []string{"TestCrashTestSuite/TestCrashes", "TestCrashTestSuite"} {
			if v := streamVerdict(r.stream, path); v != "" {
				t.Errorf("crash %s: %s got the verdict %q, want none: the end was put on the test that was running\n%s", crash, path, v, out)
			}
		}
		if crash == "kill" && !strings.Contains(r.stderr, "gotest: suite binary terminated by signal") {
			t.Errorf("crash %s: stderr does not name the signal\n%s", crash, out)
		}
	}
}

// A method that panics fails itself, its suite and its package, and the
// verdicts testing reported as the panic unwound stay in the stream.
func (s *CanaryRelayTestSuite) TestAPanickingMethodKeepsItsVerdicts(t *gotest.T) {
	r := s.runWith(t, "relay", t.TempDir(), []string{"RELAY_CRASH=method"}, "-run=^TestCrashTestSuite$")
	out := r.stream + r.stderr
	if r.code != 1 {
		t.Errorf("method panic: exit code %d, want 1\n%s", r.code, out)
	}
	if v := relayPackage(r.stream); v != "fail" {
		t.Errorf("method panic: package verdict %q, want fail\n%s", v, out)
	}
	for _, path := range []string{"TestCrashTestSuite/TestCrashes", "TestCrashTestSuite"} {
		if v := streamVerdict(r.stream, path); v != "fail" {
			t.Errorf("method panic: %s verdict %q, want fail: a verdict testing reported was lost\n%s", path, v, out)
		}
	}
}
