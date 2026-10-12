package migrate

// Pattern splitting, for both path separators on any host.
var (
	CutWildcard   = cutWildcard
	IsPathPattern = isPathPattern
)

// TokenStarts maps lines inside multi-line literals to their first line.
var TokenStarts = tokenStarts
