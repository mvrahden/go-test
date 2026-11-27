package c_test

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"

	"fresh/c"
)

type CTestSuite struct{}

func (s *CTestSuite) TestReads(t *gotest.T) {
	gotest.NotEmpty(t, c.Data)
	for _, f := range []string{"testdata/fixture.txt", "../notes/outside.txt"} {
		data, err := os.ReadFile(f)
		gotest.NoError(t, err)
		gotest.NotEmpty(t, data)
	}
}
