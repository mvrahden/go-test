//go:build !unix

package canary_test //nolint:fail-guard // ring 0: raw checks only, never a gotest assertion

// suiteSignallable: Windows has no signal for a single process.
const suiteSignallable = false
