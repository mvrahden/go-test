//go:build !unix

package canary_test //nolint:fail-guard

// suiteSignallable: Windows has no signal for a single process.
const suiteSignallable = false
