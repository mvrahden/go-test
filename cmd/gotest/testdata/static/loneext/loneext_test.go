package loneext_test

import (
	"github.com/mvrahden/go-test/cmd/gotest/testdata/static/loneext"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type NameTestSuite struct{}

func (s *NameTestSuite) TestName(t *gotest.T) {
	gotest.Equal(t, "loneext", loneext.Name())
}
