package lint

import (
	"go/token"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// genCollectors share one source loader per file set, so a dependency's
// source loads once per lint run.
var genCollectors sync.Map // *token.FileSet → genCollector

type genCollector interface {
	CollectSuiteSpecs(*packages.Package) gotestgen.CollectorResult
	Rejections(*packages.Package, gotestgen.CollectorResult) []gotestgen.CollectorError
}

// reportedLines holds, per pass, the lines another rule found something on,
// suppressed or not.
var reportedLines sync.Map // *analysis.Pass → map[token.Position]bool

func reportedLine(pass *analysis.Pass, rule Rule, pos token.Pos) {
	if rule == GenerationRejects || (ruleMeta[rule].Tier != TierIntegrity && skipped(rule)) {
		return
	}
	lines, _ := reportedLines.LoadOrStore(pass, map[token.Position]bool{})
	lines.(map[token.Position]bool)[lineOf(pass.Fset, pos)] = true
}

func lineOf(fset *token.FileSet, pos token.Pos) token.Position {
	p := fset.Position(pos)
	return token.Position{Filename: p.Filename, Line: p.Line}
}

// checkGenerationRejects reports what the generator rejects in this package,
// at the position it rejects it, with its message. A line another rule
// reported on is left to that rule.
func checkGenerationRejects(pass *analysis.Pass) {
	lines, _ := reportedLines.LoadAndDelete(pass)
	claimed, _ := lines.(map[token.Position]bool)
	if len(pass.Files) == 0 {
		return
	}
	pkg := &packages.Package{
		ID:        pass.Pkg.Path(),
		Name:      pass.Pkg.Name(),
		PkgPath:   pass.Pkg.Path(),
		Fset:      pass.Fset,
		Syntax:    pass.Files,
		Types:     pass.Pkg,
		TypesInfo: pass.TypesInfo,
	}
	c, _ := genCollectors.LoadOrStore(pass.Fset, gotestgen.NewSourceCollector(pass.Fset))
	col := c.(genCollector)
	result := col.CollectSuiteSpecs(pkg)
	rejected := col.Rejections(pkg, result)
	if len(rejected) == 0 {
		rejected = gotestgen.FuzzRejections(pkg, result.Suites)
	}
	for _, ce := range rejected {
		pos := ce.Pos
		if !pos.IsValid() {
			pos = pass.Files[0].Name.Pos()
		}
		if claimed[lineOf(pass.Fset, pos)] {
			continue
		}
		report(pass, GenerationRejects, pos, "%s", ce.Err.Error())
	}
}
