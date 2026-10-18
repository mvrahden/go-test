package gotest

import "context"

func ExportTCtx(t *T) context.Context { return t.ctx }

// ExportExplodeSeeds and ExportSeeds expose F's buffered-seed logic without
// a *testing.F, which has no public constructor outside a real fuzz target.
func ExportExplodeSeeds(f *F, explode func(seed []any) ([]any, error)) ([][]any, error) {
	return f.explodeSeeds(explode)
}
func ExportSeeds(f *F) [][]any { return f.seeds }

var (
	ExportIsExternalPackage = isExternalPackage
	ExportSplitTestName     = splitTestName
	ExportSnapshotReadonly  = snapshotReadonly
	ExportPkgCache          = &pkgCache
)

var ExportDefaultSnapshotDir = defaultSnapshotDir

// ExportRedirectSnapshots makes MatchSnapshot read and write its snapshots in
// dir until the returned restore runs.
func ExportRedirectSnapshots(dir string) (restore func()) {
	prev := snapshotDirFor
	snapshotDirFor = func(string) string { return dir }
	return func() { snapshotDirFor = prev }
}

// ExportMatchSnapshotFromPtest calls MatchSnapshot from this internal test
// file, so caller-package detection sees a ptest caller and picks no suffix.
func ExportMatchSnapshotFromPtest(t testingT, value any) { MatchSnapshot(t, value) }

// ExportCompiledPatterns counts the patterns Regexp has compiled and kept.
func ExportCompiledPatterns() int {
	n := 0
	compiledPatterns.Range(func(any, any) bool { n++; return true })
	return n
}
