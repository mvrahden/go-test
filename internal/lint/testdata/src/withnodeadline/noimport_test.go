package withnodeadline

// Without a gotest import the file cannot name NoDeadline, so the finding
// carries no fix.
func noImport() Cfg {
	return Cfg{Timeout: -1} // want `SuiteConfig.Timeout is negative`
}
