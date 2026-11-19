package perf_test

// The budgets the perf suites hold a run of the staged module to.
const (
	// goTestCPerGroup: go test -c calls equal the groups of binary names
	// (true) or the packages (false).
	goTestCPerGroup = false
	// goCallsPerExtraPackage: go invocations a warm run adds per package
	// beyond the first six.
	goCallsPerExtraPackage = 1
	// testVariantExportLoads: go list calls carrying both -test=true and
	// -export=true, which compile every test variant.
	testVariantExportLoads = 1
	// execsPerSuite: test-binary starts per dispatched suite, text mode.
	execsPerSuite = 1

	// rerunCompiles, rerunLinks, rerunVets: build actions of a warm run
	// after nothing changed.
	rerunCompiles = 0
	rerunLinks    = 0
	rerunVets     = 0
	// editCompiles: after one test-file edit, the package with its tests and
	// the test main; the staged packages have no external tests.
	editCompiles = 2
	// editLinks, editVets: the edited package's binary and its vet.
	editLinks = 1
	editVets  = 1
)
