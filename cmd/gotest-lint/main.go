package main

import (
	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(lint.Analyzer)
}
