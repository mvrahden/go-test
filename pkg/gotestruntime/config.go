package gotestruntime

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// EnvTimeoutScale multiplies every suite and fixture duration the harness
// runs under, for a machine slower than the one the budgets were written on.
const EnvTimeoutScale = "GOTEST_TIMEOUT_SCALE"

// timeoutScale reads EnvTimeoutScale once: 1 when unset or empty, an error
// for anything but a number above zero. It then unsets it: the scale is the
// run's, not that of the processes its tests start.
var timeoutScale = sync.OnceValues(func() (float64, error) {
	v := os.Getenv(EnvTimeoutScale)
	_ = os.Unsetenv(EnvTimeoutScale)
	if v == "" {
		return 1, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || !(f > 0) || f > 1e6 {
		return 1, fmt.Errorf("%s=%q is not a number above zero", EnvTimeoutScale, v)
	}
	return f, nil
})

// scaleTimeout is d multiplied by GOTEST_TIMEOUT_SCALE. Zero and negative
// durations (gotest.NoDeadline) stay as they are.
func scaleTimeout(d time.Duration) time.Duration {
	f, _ := timeoutScale()
	if d <= 0 || f == 1 {
		return d
	}
	return time.Duration(float64(d) * f)
}

// WithSuiteDefaults is the config a SuiteConfig() marker runs under: a
// duration it leaves at zero takes DefaultSuiteConfig's, as if the marker were
// absent, and every duration is scaled by GOTEST_TIMEOUT_SCALE. A negative
// duration (gotest.NoDeadline) stays and disables the deadline. The declared
// value itself remains the verdict budget, so a zero is never enforced,
// exactly like a suite without a marker.
func WithSuiteDefaults(declared gotest.SuiteConfig) gotest.SuiteConfig {
	def := gotest.DefaultSuiteConfig()
	if declared.Timeout == 0 {
		declared.Timeout = def.Timeout
	}
	if declared.SetupTimeout == 0 {
		declared.SetupTimeout = def.SetupTimeout
	}
	return scaleSuite(declared)
}

// WithFixtureDefaults is [WithSuiteDefaults] for FixtureConfig() and
// SharedFixtureConfig() markers: a zero Timeout takes DefaultFixtureConfig's.
func WithFixtureDefaults(declared gotest.FixtureConfig) gotest.FixtureConfig {
	if declared.Timeout == 0 {
		declared.Timeout = gotest.DefaultFixtureConfig().Timeout
	}
	declared.Timeout = scaleTimeout(declared.Timeout)
	return declared
}

func scaleSuite(c gotest.SuiteConfig) gotest.SuiteConfig {
	c.Timeout = scaleTimeout(c.Timeout)
	c.SetupTimeout = scaleTimeout(c.SetupTimeout)
	return c
}
