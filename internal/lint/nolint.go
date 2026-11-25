package lint

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
)

// checkNolint reports each id a nolint comment names that is neither a
// gotest rule, "all", nor a golangci-lint linter, and each comment that
// reaches a gotest rule with no reason. It reports past nolint comments: the
// comment at fault must not silence its own finding.
func checkNolint(pass *analysis.Pass) {
	if cfg.disableNolint {
		return
	}
	allowed := nolintAllowed()
	for _, file := range pass.Files {
		if isGeneratedFile(pass, file.Pos()) || ast.IsGenerated(file) {
			continue
		}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				ids, reason, ok := nolintIDs(c.Text)
				for _, id := range ids {
					if id == "all" || Known(Rule(id)) || foreignLinters[id] || allowed[id] {
						continue
					}
					pass.Report(analysis.Diagnostic{
						Pos:      c.Pos(),
						Category: string(NolintUnknown),
						Message:  "unknown rule " + id + " in nolint comment",
					})
				}
				if ok && reason == "" && reachesGotest(ids) {
					pass.Report(analysis.Diagnostic{
						Pos:      c.Pos(),
						End:      c.End(),
						Category: string(NolintReason),
						Message:  "nolint comment gives no reason; write one after it, as //nolint:<rule> // <reason>",
					})
				}
			}
		}
	}
}

// reachesGotest reports whether a nolint comment naming ids can suppress a
// gotest rule: it names none, all, or one of them.
func reachesGotest(ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == "all" || Known(Rule(id)) {
			return true
		}
	}
	return false
}

// usedDirectives holds, per pass, the nolint comments that suppressed a
// finding.
var usedDirectives sync.Map // *analysis.Pass → map[token.Pos]bool

func noteUsed(pass *analysis.Pass, by []*ast.Comment) {
	used, _ := usedDirectives.LoadOrStore(pass, map[token.Pos]bool{})
	for _, c := range by {
		used.(map[token.Pos]bool)[c.Pos()] = true
	}
}

// Directive is a nolint comment that can suppress a gotest rule.
type Directive struct {
	Pos    token.Pos
	Rules  []string // the ids it names, sorted; "all" for a bare one
	Reason string
	Used   bool // it suppressed a finding of this pass
}

// passResult is what the analyzer hands its driver for a package.
type passResult struct {
	Directives []Directive
}

// directives lists the pass's gotest nolint comments, generated files left
// out, and clears what the pass recorded about them.
func directives(pass *analysis.Pass) *passResult {
	v, _ := usedDirectives.LoadAndDelete(pass)
	used, _ := v.(map[token.Pos]bool)
	res := &passResult{}
	for _, file := range pass.Files {
		if isGeneratedFile(pass, file.Pos()) || ast.IsGenerated(file) {
			continue
		}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				ids, reason, ok := nolintIDs(c.Text)
				if !ok || !reachesGotest(ids) {
					continue
				}
				rules := slices.Clone(ids)
				if len(rules) == 0 {
					rules = []string{"all"}
				}
				slices.Sort(rules)
				res.Directives = append(res.Directives, Directive{
					Pos:    c.Pos(),
					Rules:  slices.Compact(rules),
					Reason: reason,
					Used:   used[c.Pos()],
				})
			}
		}
	}
	return res
}

// nolintAllowed is the set -nolint-allow names: linters lint does not know
// that a nolint comment may name, such as a module plugin.
func nolintAllowed() map[string]bool {
	m := map[string]bool{}
	for name := range strings.SplitSeq(cfg.nolintAllow, ",") {
		if name = strings.TrimSpace(name); name != "" {
			m[name] = true
		}
	}
	return m
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
