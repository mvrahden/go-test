package gotest

import "context"

func ExportTCtx(t *T) context.Context { return t.ctx }

// ExportEncodeSeeds and ExportSeedMismatch expose F's seed-encoding and
// seed/target type-agreement logic without a *testing.F, which has no public
// constructor outside a real fuzz target.
func ExportEncodeSeeds(f *F, args []any) []any { return f.encodeSeeds(args) }
func ExportSeedMismatch(f *F, want int) int    { return f.seedMismatch(want) }
