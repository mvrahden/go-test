package lintfocusunresolved

import "github.com/mvrahden/go-test/pkg/gotest"

type F_UnresolvedTestSuite struct{}

func (s *F_UnresolvedTestSuite) TestOne(t *gotest.T) {}
