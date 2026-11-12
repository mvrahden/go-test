package lintdroppedcomment

import "github.com/mvrahden/go-test/pkg/gotest"

type Harness struct{}

func (h *Harness) TestConnection(t *gotest.T) {}

type InTestSuite struct{}

func (s *InTestSuite) TestX(t *gotest.T) {}
