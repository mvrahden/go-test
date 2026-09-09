module fuzzcrash

go 1.25.0

require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000

// Rewritten to the checkout under test when the module is staged.
replace github.com/mvrahden/go-test => REPO_ROOT
