//go:build !unix

package lifecycle_test

// suiteSignallable: Windows has no signal for a single process.
const suiteSignallable = false
