package drill

var ExportLockError = lockError

// ExportSetLockOpened runs f between a lock file's open and its lock.
func ExportSetLockOpened(f func(path string)) { lockOpened = f }

// ExportSetRemoveScratch replaces how a prune removes a scratch.
func ExportSetRemoveScratch(f func(path string) error) { removeScratch = f }

// ExportResetSeams undoes the setters above.
func ExportResetSeams() {
	lockOpened = func(string) {}
	removeScratch = removeAll
}

var ExportRemoveAll = removeAll
