package about

import "runtime"

func ShortInfo() string {
	return Application + " (" + Repo + ")"
}

func LongInfo() string {
	return Application + " " + Version + " " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}
