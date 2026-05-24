package gotest

import "context"

func ExportTCtx(t *T) context.Context { return t.ctx }
