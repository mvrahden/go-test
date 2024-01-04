package about

import "runtime"

var (
	GoVersion = runtime.Version()
	GoArch    = runtime.GOARCH
	GoOS      = runtime.GOOS
)
