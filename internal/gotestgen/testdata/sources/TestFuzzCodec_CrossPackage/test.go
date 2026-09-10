package testpkg

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"testpkg/TestFuzzCodec_CrossDep"
)

type Envelope struct {
	Name string
	S    crossdep.Setting
	P    *crossdep.Setting
	Tag  crossdep.ID
}

type CrossPkgFuzzTestSuite struct{}

func (s *CrossPkgFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *CrossPkgFuzzTestSuite) FuzzEnvelope(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, e Envelope) {
		gotest.True(t, e.S.Value == e.S.Value)
	})
}
