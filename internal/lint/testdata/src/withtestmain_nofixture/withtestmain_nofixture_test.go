package withtestmain_nofixture

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// No suite binds a fixture: there is nothing to tear down, and m.Run is fine.
type PlainTestSuite struct{}

func (s *PlainTestSuite) TestRead(t *gotest.T) {}

func TestMain(m *testing.M) { os.Exit(m.Run()) }
