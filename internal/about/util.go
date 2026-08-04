package about

import (
	"runtime"
	"runtime/debug"
)

func ShortInfo() string {
	return Application + " (" + Repo + ")"
}

func LongInfo() string {
	return Application + " " + ResolvedVersion() + " " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}

// ResolvedVersion returns the release version. The ldflags-stamped Version
// wins; otherwise the module version from build info is used, so
// tool-directive builds (`go tool gotest`) report the version pinned in the
// consumer's go.mod instead of "dev". Builds from a replace directive or a
// source checkout are marked explicitly — consumers use this to apply
// version-gated guidance.
func ResolvedVersion() string {
	bi, ok := debug.ReadBuildInfo()
	return resolveVersion(Version, bi, ok)
}

func resolveVersion(stamped string, bi *debug.BuildInfo, ok bool) string {
	if stamped != "dev" || !ok {
		return stamped
	}
	if bi.Main.Replace != nil {
		return "dev (replace directive)"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return "dev (source checkout)"
}
