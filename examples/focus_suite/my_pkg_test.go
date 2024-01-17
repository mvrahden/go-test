package focussuite_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type NoopTestSuite struct{}
type X_ExcludedTestSuite struct{}
type F_FocusedTestSuite struct{}

func (ts *F_FocusedTestSuite) TestSomething(t *gotest.T) {}
