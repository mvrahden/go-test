package gotest

import (
	"context"

	"github.com/mvrahden/go-test/internal/runstate"
)

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

// ExportMatchSnapshotFromPtest calls MatchSnapshot from this internal test
// file, so caller-package detection sees a ptest caller and picks no suffix.
func ExportMatchSnapshotFromPtest(t testingT, value any) { MatchSnapshot(t, value) }

// ExportExecTestFn runs fn the way It and When run their bodies.
func ExportExecTestFn(fn func(*T), it *T) { execTestFn(fn, it) }

// ExportDying reports the dying mark; ExportResetDying clears it.
func ExportDying() bool { return runstate.Dying() }
func ExportResetDying() { runstate.Reset() }
