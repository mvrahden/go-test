//go:build !unix

package quick

import "context"

// runPipeline: SIGPIPE is a Unix matter.
func runPipeline(context.Context) (string, error) { return "y", nil }
