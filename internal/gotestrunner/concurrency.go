package gotestrunner

// ComputeConcurrency determines inter-suite process count and per-suite
// -test.parallel value from a total concurrency budget.
//
// budget is the total concurrent test methods target (0 → 2*gomaxprocs).
// numSuites is the number of suite targets to dispatch.
// gomaxprocs is the GOMAXPROCS value (used to cap process count).
//
// The inter value caps at gomaxprocs to avoid creating more OS processes
// than CPU cores. Remaining budget flows to intra (goroutine-level
// parallelism within each process), which is cheaper.
func ComputeConcurrency(budget, numSuites, gomaxprocs int) (inter, intra int) {
	if gomaxprocs <= 0 {
		gomaxprocs = 1
	}
	if budget <= 0 {
		budget = 2 * gomaxprocs
	}
	if numSuites <= 0 {
		return 1, budget
	}
	inter = min(numSuites, gomaxprocs, budget)
	if inter <= 0 {
		inter = 1
	}
	intra = max(1, budget/inter)
	return inter, intra
}
