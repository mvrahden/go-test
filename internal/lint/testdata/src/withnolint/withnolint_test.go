package withnolint

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	_ "github.com/stretchr/testify/assert"  //nolint:testify
	_ "github.com/stretchr/testify/require" // want `testify import github.com/stretchr/testify/require — consider migrating to gotest`
)

// suppressed: inline on same line
func TestInline(t *testing.T) {} //nolint:stdlib-test

// suppressed: nolint directly above
//
//nolint:stdlib-test
func TestAbove(t *testing.T) {}

// suppressed: nolint in doc block
// TestDocBlock tests doc-block suppression.
//
//nolint:stdlib-test
func TestDocBlock(t *testing.T) {}

// suppressed: blanket nolint
// nolint
func TestBlanket(t *testing.T) {}

// suppressed: multiple rules including stdlib-test
//
//nolint:testify,stdlib-test
func TestMultiRule(t *testing.T) {}

// NOT suppressed: wrong rule
func TestWrongRule(t *testing.T) {} //nolint:testify // want `stdlib test TestWrongRule — consider using a gotest suite`

// NOT suppressed: no nolint at all
func TestUnsuppressed(t *testing.T) {} // want `stdlib test TestUnsuppressed — consider using a gotest suite`

// suppressed: nolint with reason
func TestWithReason(t *testing.T) {} //nolint:stdlib-test // legacy test

// suppressed: fail-guard inline
func TestFailGuardNolint(t *testing.T) { //nolint:stdlib-test
	var err error
	if err != nil { //nolint:fail-guard
		gotest.Fail(t, "boom")
	}
}
