package lint

import "github.com/mvrahden/go-test/internal/lint"

// Analyzer is the gotestlint analyzer, exported for external go/analysis
// drivers (multichecker-style integration; there is no golangci-lint plugin).
var Analyzer = lint.Analyzer
