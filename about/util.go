package about

func ShortInfo() string {
	return Application + " (" + Repo + ")"
}

func LongInfo() string {
	return Repo + " (" + GitTag + "/" + GoVersion + " " + GoOS + " " + GoArch + ")"
}
