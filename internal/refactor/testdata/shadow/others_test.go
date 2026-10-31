package shadow

import "github.com/mvrahden/go-test/pkg/gotest"

func local() int {
	ShadowTestSuite := 1
	return ShadowTestSuite
}

type named struct{}

func (named) ShadowTestSuite() {}

func (named) TestOne(t *gotest.T) {}

var (
	_ = local
	_ = (*ShadowTestSuite).TestOne
	_ = (*OtherTestSuite).TestOne
	_ = named.TestOne
	_ = OtherTestSuite{ShadowTestSuite: 1}
)
