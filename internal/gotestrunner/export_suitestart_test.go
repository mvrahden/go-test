package gotestrunner

import (
	"context"
	"sync/atomic"
)

// ExportCountSuiteStarts counts suite process starts, calling after with the
// count once each start returns, and returns what restores the real start.
func ExportCountSuiteStarts(after func(n int)) (restore func()) {
	var n atomic.Int32
	suiteStart = func(ctx context.Context, target SuiteTarget, env []string, test2json bool) (startedSuite, error) { //nolint:gocritic // hugeParam: as startSuite
		s, err := startSuite(ctx, target, env, test2json)
		after(int(n.Add(1)))
		return s, err
	}
	return func() { suiteStart = startSuite }
}
