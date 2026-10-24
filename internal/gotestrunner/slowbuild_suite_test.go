package gotestrunner_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowBuildThresholdTestSuite: GOTEST_SLOW_BUILD_THRESHOLD moves the
// slow-build notice.
//
// Sequential: Setenv.
type SlowBuildThresholdTestSuite struct{}

func (s *SlowBuildThresholdTestSuite) TestTheVariableSetsTheThreshold(t *gotest.T) {
	entries := []struct {
		Desc  string
		value string
		want  time.Duration
	}{
		{"unset", "", 15 * time.Second},
		{"a nanosecond", "1ns", time.Nanosecond},
		{"minutes", "2m", 2 * time.Minute},
		{"not a duration", "15", 15 * time.Second},
		{"zero", "0s", 15 * time.Second},
		{"negative", "-1s", 15 * time.Second},
	}
	for t, entry := range gotest.Each(t, entries) {
		t.Setenv("GOTEST_SLOW_BUILD_THRESHOLD", entry.value)
		gotest.Equal(t, entry.want, gotestrunner.ExportSlowBuildThreshold())
	}
}
