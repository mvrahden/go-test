package withskippedstyle //nolint:stdlib-test

import (
	"errors"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// With skip-fail-guard set, this expressiveness finding must stay silent.
func TestSkipped(t *testing.T) {
	err := errors.New("boom")
	if err != nil {
		gotest.Fail(t, "unexpected error")
	}
}
