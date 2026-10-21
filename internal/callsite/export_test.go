package callsite

import "runtime/debug"

// ExportStdPath judges a function by import path alone, as under -trimpath.
func ExportStdPath(fn string) bool { return stdPath(fn, builtModules()) }

// ExportStdPathIn judges fn as a build holding the dotless main module main
// would, under -trimpath.
func ExportStdPathIn(fn, main string) bool {
	return stdPath(fn, modulesOf(&debug.BuildInfo{Main: debug.Module{Path: main}}))
}

// ExportStdPathInBuild judges fn as a build info records would, under
// -trimpath.
func ExportStdPathInBuild(fn string, info *debug.BuildInfo) bool {
	return stdPath(fn, modulesOf(info))
}
