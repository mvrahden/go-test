// Command lintvettool is the go vet -vettool wrapper around gotest's analyzer.
package main

import (
	"github.com/mvrahden/go-test/pkg/lint"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() { unitchecker.Main(lint.Analyzer) }
