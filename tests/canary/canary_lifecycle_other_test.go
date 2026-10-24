//go:build !unix

package canary_test //nolint:fail-guard

import "github.com/mvrahden/go-test/pkg/gotest"

// suiteSignallable: Windows has no signal for a single process.
const suiteSignallable = false

func signalSuite(*gotest.T, string) {}
