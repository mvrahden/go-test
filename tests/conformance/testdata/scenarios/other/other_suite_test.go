package other

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type OtherTestSuite struct{}

func (s *OtherTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *OtherTestSuite) TestUnbound(t *gotest.T) {
	t.When("nothing states it", func(w *gotest.T) {
		w.It("fails when run", func(it *gotest.T) {
			gotest.Equal(it, 3, Two())
		})
	})
}
