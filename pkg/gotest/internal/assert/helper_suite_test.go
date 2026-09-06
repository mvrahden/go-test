// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

// HelperTestSuite covers the call-site tracer's contract with the testing
// package: the internals it reaches into, and the frame it reports.
type HelperTestSuite struct{}

func (s *HelperTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *HelperTestSuite) BeforeEach(t *gotest.T) *ring0Ctx { return &ring0Ctx{} }

// The tracer marks frames through testing.T's unexported fields; a Go release
// that renames or retypes them must fail here, not silently degrade locations.
func (s *HelperTestSuite) TestGoTestingInternalsCompatible(t *gotest.T, _ *ring0Ctx) {
	v := reflect.ValueOf(t.T()).Elem()
	check := func(name string, wantType reflect.Type) {
		f := v.FieldByName(name)
		if !f.IsValid() {
			fatalf(t, "testing.T missing field %q: Go internals changed", name)
		}
		if !f.CanAddr() {
			fatalf(t, "testing.T field %q is not addressable", name)
		}
		if f.Type() != wantType {
			fatalf(t, "testing.T field %q type changed: want %v, got %v", name, wantType, f.Type())
		}
	}
	check("mu", reflect.TypeFor[sync.RWMutex]())
	check("helperPCs", reflect.TypeFor[map[uintptr]struct{}]())
	check("helperNames", reflect.TypeFor[map[string]struct{}]())
}

func (s *HelperTestSuite) TestSkipInternalFrames_NilT_NoPanic(t *gotest.T, _ *ring0Ctx) {
	assert.SkipInternalFrames(nil)
}

func (s *HelperTestSuite) TestSkipInternalFrames_MarksCallerAsHelper(t *gotest.T, _ *ring0Ctx) {
	sub := testing.T{}
	assert.SkipInternalFrames(&sub)
}

func helperThatCallsCallerFrame() string {
	return assert.CallerFrame()
}

func (s *HelperTestSuite) TestCallerFrame_DirectCall_ReturnsFrame(t *gotest.T, _ *ring0Ctx) {
	frame := assert.CallerFrame()
	if frame == "" {
		fatalf(t, "expected non-empty frame, got empty")
	}
	if !strings.Contains(frame, "helper_suite_test.go:") {
		fatalf(t, "expected 'helper_suite_test.go:' in frame, got: %q", frame)
	}
	if strings.Contains(frame, "called from") {
		fatalf(t, "CallerFrame should not contain 'called from', got: %q", frame)
	}
}

func (s *HelperTestSuite) TestCallerFrame_ThroughHelper_ReturnsOutermostUserFrame(t *gotest.T, _ *ring0Ctx) {
	frame := helperThatCallsCallerFrame()
	if frame == "" {
		fatalf(t, "expected non-empty frame, got empty")
	}
	if !strings.Contains(frame, "helper_suite_test.go:") {
		fatalf(t, "expected 'helper_suite_test.go:' in frame, got: %q", frame)
	}
}
