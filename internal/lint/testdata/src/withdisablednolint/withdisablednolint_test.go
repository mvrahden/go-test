package withdisablednolint

import (
	"testing"

	_ "github.com/stretchr/testify/assert" //nolint:testify // want `testify import github.com/stretchr/testify/assert — consider migrating to gotest`
)

func TestInline(t *testing.T) {} //nolint:stdlib-test // want `stdlib test TestInline — consider using a gotest suite`

//nolint:stdlib-test
func TestAbove(t *testing.T) {} // want `stdlib test TestAbove — consider using a gotest suite`

//nolint
func TestBlanket(t *testing.T) {} // want `stdlib test TestBlanket — consider using a gotest suite`

//nolint:stdlib-test
// TestDocBlock verifies doc-block suppression is bypassed.
func TestDocBlock(t *testing.T) {} // want `stdlib test TestDocBlock — consider using a gotest suite`
