package testkit

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// Scaled is d multiplied by GOTEST_TIMEOUT_SCALE, as the harness scales every
// budget. It is for the bounds a test body measures itself, such as how long
// a stop took, so a bound written for one machine holds on a slower one. Scale
// only a bound whose scaled value still tells the outcomes apart: an upper
// bound that must stay below a timeout the test also sets is left as written.
// Zero and negative durations stay as they are.
func Scaled(d time.Duration) time.Duration {
	return gotestruntime.ScaleDuration(d)
}
