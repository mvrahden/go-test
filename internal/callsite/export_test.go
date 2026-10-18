package callsite

// ExportStdPath judges a function by import path alone, as under -trimpath.
func ExportStdPath(fn string) bool { return stdPath(fn, mainModule()) }

var ExportStdPathIn = stdPath
