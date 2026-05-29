package lint

import "github.com/mvrahden/go-test/internal/lint"

// Analyzer is the gotestlint analyzer for integration with
// external analysis drivers such as golangci-lint.
var Analyzer = lint.Analyzer
