//go:build race

package gotestgen_test

// raceEnabled mirrors the -race setting of this test binary onto the child
// processes spawned by ParallelLifecycleTestSuite, so the generated harness is
// exercised under the race detector whenever this package is.
const raceEnabled = true
