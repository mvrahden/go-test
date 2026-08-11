package withfailguard_noimport //nolint:stdlib-test

import (
	"errors"
	"testing"
)

// Without a gotest import the file has not adopted the framework — halting
// guards here are idiomatic stdlib Go and none of fail-guard's business.
func TestNoImport(t *testing.T) {
	err := errors.New("boom")
	if err != nil {
		t.Fatal(err)
	}
}
