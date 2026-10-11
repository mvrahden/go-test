// Command fixdriver is the go/analysis driver over the gotest analyzer, the
// oracle Fix's parity test compares its output against.
package main

import (
	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(lint.Analyzer) }
