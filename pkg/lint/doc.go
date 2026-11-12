// Package lint exports gotest's analyzer, gotestlint, for go/analysis
// drivers other than gotest lint.
//
// gotest lint is the reference driver: it alone renders GitHub annotations,
// reads lint.skip from .gotest.yml and refuses a -skip of an integrity rule.
//
// go vet runs the analyzer through a vettool, a main around it:
//
//	package main
//
//	import (
//		"github.com/mvrahden/go-test/pkg/lint"
//		"golang.org/x/tools/go/analysis/unitchecker"
//	)
//
//	func main() { unitchecker.Main(lint.Analyzer) }
//
// tests/conformance/testdata/lintvettool/main.go in gotest's repository is
// one. Build it and pass it to go vet; the analyzer's flags take its name:
//
//	go build -o gotestvet ./tools/gotestvet
//	go vet -vettool=$(pwd)/gotestvet -gotestlint.skip-testify ./...
//
// golangci-lint runs it as a module plugin, the nested module
// github.com/mvrahden/go-test/pkg/lint/golangci. Name it in .custom-gcl.yml:
//
//	plugins:
//	  - module: github.com/mvrahden/go-test/pkg/lint/golangci
//	    version: <the gotest version go.mod requires>
//
// build the binary with golangci-lint custom, and enable the linter in
// .golangci.yml:
//
//	linters:
//	  enable: [gotestlint]
//	  settings:
//	    custom:
//	      gotestlint:
//	        type: module
//	        settings:
//	          skip: [testify]
//	          nolint-allow: [nilaway]
//
// The plugin's version must be the gotest version the module requires:
// the generation-rejects rule runs gotest's generator, which must be the
// one that generates the module's harness.
//
// Both drivers hand the analyzer one test package of a directory at a
// time. dropped-case, which judges whether anything runs a case, needs both
// the internal and the external test package: gotest lint loads them
// together and agrees with the generator; under go vet and golangci-lint the
// rule reports only in a directory with no external test package and leaves
// the rest to the generator's refusal.
//
// Under golangci-lint, //nolint:gotestlint silences the whole linter and
// //nolint:<rule> one of its rules; nolintlint does not know the rule ids,
// so tell it about them (allow-unused, or its exclusions) where it runs.
package lint
