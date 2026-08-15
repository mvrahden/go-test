package runtimebehaviors

import "github.com/mvrahden/go-test/pkg/gotest"

// Behaviors that cannot be read from source: one guarded by a condition and
// one whose description is computed. The walker must report the method as
// incomplete rather than presenting the behaviors it *can* see as the whole
// specification.
type RuntimeBehaviorsTestSuite struct{}

func (s *RuntimeBehaviorsTestSuite) TestConditional(t *gotest.T) {
	enabled := len("always true") > 0

	t.When("the feature flag decides what is specified", func(t *gotest.T) {
		t.It("always states this one", func(t *gotest.T) {
			gotest.True(t, enabled)
		})
		if enabled {
			t.It("only states this one when enabled", func(t *gotest.T) {
				gotest.True(t, enabled)
			})
		}
	})
}
