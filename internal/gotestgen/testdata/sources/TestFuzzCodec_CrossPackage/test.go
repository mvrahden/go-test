package testpkg

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"testpkg/TestFuzzCodec_CrossDep"
)

type Envelope struct {
	Name string
	S    crossdep.Setting
	P    *crossdep.Setting
}

type CrossPkgFuzzTestSuite struct{}

func (s *CrossPkgFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *CrossPkgFuzzTestSuite) FuzzEnvelope(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, e Envelope) {
		gotest.True(t, e.S.Value == e.S.Value)
	})
}
