package lintfixbad

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ValueTestSuite struct{}

func (s ValueTestSuite) TestValue(t *gotest.T) {}

// With a pointer receiver this method value no longer compiles.
var _ = ValueTestSuite{}.TestValue

func TestPlain(t *testing.T) {
	var err error
	gotest.True(t, err == nil)
}
