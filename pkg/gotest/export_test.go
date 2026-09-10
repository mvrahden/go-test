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
	ExportReadAndRestore    = readAndRestore
	ExportPkgCache          = &pkgCache
)

// ExportMatchSnapshotFromPtest calls MatchSnapshot from this internal test
// file, so caller-package detection sees a ptest caller and picks no suffix.
func ExportMatchSnapshotFromPtest(t testingT, value any) { MatchSnapshot(t, value) }
