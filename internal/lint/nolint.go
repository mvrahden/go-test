package lint

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// checkNolintIDs reports each id a nolint comment names that is neither a
// gotest rule, "all", nor a golangci-lint linter. It reports past nolint
// comments: the comment at fault must not silence its own finding.
func checkNolintIDs(pass *analysis.Pass) {
	if cfg.disableNolint {
		return
	}
	for _, file := range pass.Files {
		if isGeneratedFile(pass, file.Pos()) || ast.IsGenerated(file) {
			continue
		}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				ids, _ := nolintIDs(c.Text)
				for _, id := range ids {
					if id == "all" || Known(Rule(id)) || foreignLinters[id] {
						continue
					}
					pass.Report(analysis.Diagnostic{
						Pos:      c.Pos(),
						Category: string(NolintUnknown),
						Message:  "unknown rule " + id + " in nolint comment",
					})
				}
			}
		}
	}
}

// foreignLinters are golangci-lint's linter and formatter names, current and
// retired: nolint comments are shared with it, and its ids are its own.
var foreignLinters = func() map[string]bool {
	m := map[string]bool{}
	for _, id := range []string{
		"arangolint", "asasalint", "asciicheck", "bidichk", "bodyclose", "canonicalheader",
		"containedctx", "contextcheck", "copyloopvar", "cyclop", "decorder", "depguard",
		"dogsled", "dupl", "dupword", "durationcheck", "embeddedstructfieldcheck", "err113",
		"errcheck", "errchkjson", "errname", "errorlint", "exhaustive", "exhaustruct",
		"exptostd", "fatcontext", "forbidigo", "forcetypeassert", "funcorder", "funlen",
		"ginkgolinter", "gocheckcompilerdirectives", "gochecknoglobals", "gochecknoinits",
		"gochecksumtype", "gocognit", "goconst", "gocritic", "gocyclo", "godoclint", "godot",
		"godox", "goheader", "gomoddirectives", "gomodguard", "goprintffuncname", "gosec",
		"gosmopolitan", "govet", "grouper", "iface", "importas", "inamedparam", "ineffassign",
		"interfacebloat", "intrange", "iotamixing", "ireturn", "lll", "loggercheck", "maintidx",
		"makezero", "mirror", "misspell", "mnd", "modernize", "musttag", "nakedret", "nestif",
		"nilerr", "nilnesserr", "nilnil", "nlreturn", "noctx", "noinlineerr", "nolintlint",
		"nonamedreturns", "nosprintfhostport", "paralleltest", "perfsprint", "prealloc",
		"predeclared", "promlinter", "protogetter", "reassign", "recvcheck", "revive",
		"rowserrcheck", "sloglint", "spancheck", "sqlclosecheck", "staticcheck", "tagalign",
		"tagliatelle", "testableexamples", "testifylint", "testpackage", "thelper", "tparallel",
		"unconvert", "unparam", "unqueryvet", "unused", "usestdlibvars", "usetesting",
		"varnamelen", "wastedassign", "whitespace", "wrapcheck", "wsl", "wsl_v5", "zerologlint",
		// formatters
		"gci", "gofmt", "gofumpt", "goimports", "golines", "swaggo",
		// retired names old comments still carry
		"deadcode", "exhaustivestruct", "execinquery", "exportloopref", "gas", "goerr113",
		"golint", "gomnd", "gosimple", "ifshort", "interfacer", "maligned", "megacheck",
		"nosnakecase", "scopelint", "structcheck", "stylecheck", "tenv", "typecheck",
		"varcheck", "vet", "vetshadow",
	} {
		m[id] = true
	}
	return m
}()
