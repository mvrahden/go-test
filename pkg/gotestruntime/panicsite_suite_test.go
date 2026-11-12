// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestruntime_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/internal/carry"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// PanicSiteTestSuite: what a method's panic carries when AfterEach re-raises it.
type PanicSiteTestSuite struct{}

func (s *PanicSiteTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *PanicSiteTestSuite) TestAMethodPanicCarriesNoSecondStack(t *gotest.T) {
	var got any
	func() {
		defer func() { got = recover() }()
		func() {
			defer gotestruntime.AfterEach(func() {})
			panic("boom")
		}()
	}()
	p, ok := got.(*carry.Panic)
	if !ok {
		fatalf(t, "AfterEach re-raised a %T, want the carried panic", got)
	}
	if len(p.Stack()) != 0 {
		t.Errorf("the panic carries a stack of its own beside the runtime's traceback:\n%s", p.Stack())
	}
	text := p.String()
	if !strings.HasPrefix(text, "boom\n\n[panicked at panicsite_suite_test.go:") || !strings.HasSuffix(text, "; recovered from the test method, ahead of its AfterEach]") {
		t.Errorf("the panic does not name the line it happened at: %q", text)
	}
}
