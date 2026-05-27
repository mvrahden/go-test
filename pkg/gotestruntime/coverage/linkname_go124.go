//go:build !go1.25 && !gotest_no_coverage_intercept

package coverage

import _ "unsafe"

//go:linkname testingCover testing.cover2
var testingCover coverState
