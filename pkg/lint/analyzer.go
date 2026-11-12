package lint

import "github.com/mvrahden/go-test/internal/lint"

// Analyzer is the gotestlint analyzer, exported for go/analysis drivers
// other than gotest lint; the package documentation names them.
var Analyzer = lint.Analyzer
