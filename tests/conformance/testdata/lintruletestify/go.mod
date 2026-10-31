module lintruletestifymod

go 1.26.0

require (
	github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.0.0
)

// Rewritten to the checkout under test when the module is staged.
replace github.com/mvrahden/go-test => REPO_ROOT

replace github.com/stretchr/testify => ./testify
