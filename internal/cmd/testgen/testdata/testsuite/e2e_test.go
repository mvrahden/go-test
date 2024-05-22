package e2epkg

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyE2ETestSuite struct{}

func (m *MyE2ETestSuite) Test_HelloWorld(t *gotest.T)     {}
func (m *MyE2ETestSuite) Test_HelloWorld_1(t *gotest.T)   {}
func (m *MyE2ETestSuite) Test_HelloWorld_2(t *gotest.T)   {}
func (m *MyE2ETestSuite) Test_HelloWorld_3(t *gotest.T)   {}
func (m *MyE2ETestSuite) X_Test_HelloWorld_4(t *gotest.T) {}
