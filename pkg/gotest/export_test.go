package gotest

import "context"

func ExportTCtx(t *T) context.Context { return t.ctx }

// ExportExplodeSeeds and ExportSeeds expose F's buffered-seed logic without
// a *testing.F, which has no public constructor outside a real fuzz target.
func ExportExplodeSeeds(f *F, arity int, explode func(seed []any) ([]any, error)) ([][]any, error) {
	return f.explodeSeeds(arity, explode)
}
func ExportSeeds(f *F) [][]any { return f.seeds }
