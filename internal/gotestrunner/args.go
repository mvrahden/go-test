package gotestrunner

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// buildFlags are the go build flags a run hands to its compile; the
// value reports whether the flag takes one. A conventions suite holds the
// list to `go help build` of the toolchain in use. -buildvcs takes its
// value only as -buildvcs=false, so a bare one must not swallow the next
// word.
var buildFlags = map[string]bool{
	"-a": false, "-asan": false, "-asmflags": true, "-buildmode": true, "-buildvcs": false,
	"-compiler": true, "-cover": false, "-covermode": true, "-coverpkg": true, "-gccgoflags": true,
	"-gcflags": true, "-installsuffix": true, "-ldflags": true, "-linkshared": false, "-mod": true,
	"-modcacherw": false, "-modfile": true, "-msan": false, "-n": false, "-overlay": true, "-p": true,
	"-pgo": true, "-pkgdir": true, "-race": false, "-tags": true, "-toolexec": true, "-trimpath": false,
	"-work": false, "-x": false,
}

// goTestBuildFlags are go test's own flags that act before the test
// binary runs; go build does not have them.
var goTestBuildFlags = map[string]bool{"-c": false, "-exec": true, "-o": true, "-vet": true}

// buildFlagsNotTaken are the go build flags a run does not take, and why.
// The conventions suite reads it from this file.
var buildFlagsNotTaken = map[string]string{ //nolint:unused // read as source by tests/conventions
	"-C":    "it must come first on the go command line and changes its directory: run gotest from that directory",
	"-json": "under go test -json is the test event stream, which gotest writes itself",
	"-v":    "under go test -v belongs to the test binary",
}

type goFlagSide int

const (
	notGoFlag goFlagSide = iota
	buildSide            // read by the compile
	runSide              // read by the test binary
)

// classifyGoFlag reports which side of a run reads the go test flag name
// and whether it takes a value.
func classifyGoFlag(name string) (side goFlagSide, takesValue bool) {
	if v, ok := buildFlags[name]; ok {
		return buildSide, v
	}
	if v, ok := goTestBuildFlags[name]; ok {
		return buildSide, v
	}
	if v, ok := runFlags()[name]; ok {
		return runSide, v
	}
	return notGoFlag, false
}

// ClassifiedArgs holds the result of splitting go test arguments.
type ClassifiedArgs struct {
	BuildFlags  []string // flags for `go test -c`
	RunFlags    []string // flags for the test binary (without -test. prefix)
	PkgPatterns []string // package patterns like ./...
}

// ClassifyGoTestArgs splits go test arguments into build flags, run flags,
// and package patterns. Flags neither gotest nor go test knows belong to the
// test binary, as under go test: they follow the known run flags behind an
// "-args" marker, and whatever the caller wrote after its own "-args" comes
// last, verbatim.
func ClassifyGoTestArgs(args []string) ClassifiedArgs {
	var result ClassifiedArgs
	needsCoverBuild := false
	var binaryFlags, explicitArgs []string

	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "-args" {
			explicitArgs = args[i+1:]
			break
		}

		if !strings.HasPrefix(arg, "-") {
			result.PkgPatterns = append(result.PkgPatterns, arg)
			i++
			continue
		}

		name, _, hasEquals := strings.Cut(arg, "=")

		// -json is intercepted at the CLI level; drop it here to prevent
		// misclassification as a build flag.
		if name == "-json" {
			i++
			continue
		}

		side, takesValue := classifyGoFlag(name)
		if side == notGoFlag {
			// A flag nobody here declares is the test binary's. As go test
			// assumes, a non-flag after it is taken as its value.
			binaryFlags = append(binaryFlags, arg)
			i++
			if !hasEquals && i < len(args) && !strings.HasPrefix(args[i], "-") {
				binaryFlags = append(binaryFlags, args[i])
				i++
			}
			continue
		}

		dst := &result.BuildFlags
		if side == runSide {
			dst = &result.RunFlags
			needsCoverBuild = needsCoverBuild || name == "-coverprofile"
		}
		if takesValue && !hasEquals && i+1 < len(args) {
			*dst = append(*dst, arg, args[i+1])
			i += 2
			continue
		}
		*dst = append(*dst, arg)
		i++
	}

	if len(binaryFlags) > 0 || explicitArgs != nil {
		result.RunFlags = append(result.RunFlags, "-args")
		result.RunFlags = append(result.RunFlags, binaryFlags...)
		result.RunFlags = append(result.RunFlags, explicitArgs...)
	}

	if needsCoverBuild && !slices.Contains(result.BuildFlags, "-cover") {
		result.BuildFlags = append(result.BuildFlags, "-cover")
	}

	return result
}

// TranslateToTestBinaryFlags converts go test run flags to test binary flags
// by adding the -test. prefix. The "-args" marker is consumed, as go test
// consumes it, and what follows it is passed verbatim.
func TranslateToTestBinaryFlags(flags []string) []string {
	out := make([]string, 0, len(flags))
	passthrough := false
	for _, f := range flags {
		if f == "-args" && !passthrough {
			passthrough = true
			continue
		}
		if passthrough || !strings.HasPrefix(f, "-") {
			out = append(out, f)
			continue
		}
		name, val, hasEquals := strings.Cut(f, "=")
		testName := "-test." + strings.TrimPrefix(name, "-")
		if hasEquals {
			out = append(out, testName+"="+val)
		} else {
			out = append(out, testName)
		}
	}
	return out
}

// extractFlag returns the value of the last of the named flags in flags,
// stopping at -args: the flag package keeps the last occurrence.
func extractFlag(flags []string, names ...string) string {
	val := ""
	for i := 0; i < len(flags); i++ {
		f := flags[i]
		if f == "-args" {
			break
		}
		for _, name := range names {
			if v, ok := strings.CutPrefix(f, name+"="); ok {
				val = v
			} else if f == name && i+1 < len(flags) {
				i++
				val = flags[i]
			}
		}
	}
	return val
}

// stripFlag removes a named flag and its value from flags, stopping at -args.
func stripFlag(flags []string, name string) []string {
	var out []string
	for i := 0; i < len(flags); i++ {
		f := flags[i]
		if f == "-args" {
			out = append(out, flags[i:]...)
			return out
		}
		if strings.HasPrefix(f, name+"=") {
			continue
		}
		if f == name {
			i++ // skip value
			continue
		}
		out = append(out, f)
	}
	return out
}

// ExtractRunFilter returns the value of -run from run flags, if present.
func ExtractRunFilter(runFlags []string) string { return extractFlag(runFlags, "-run") }

// ExtractSkipFilter returns the value of -skip from run flags, if present.
func ExtractSkipFilter(runFlags []string) string { return extractFlag(runFlags, "-skip") }

// StripRunFilter removes -run and its value from run flags.
func StripRunFilter(runFlags []string) []string { return stripFlag(runFlags, "-run") }

// ExtractBenchFilter returns the value of -bench from run flags, if present.
func ExtractBenchFilter(runFlags []string) string { return extractFlag(runFlags, "-bench") }

// StripBenchFilter removes -bench and its value from run flags.
func StripBenchFilter(runFlags []string) []string { return stripFlag(runFlags, "-bench") }

// ExtractCoverProfile returns the value of -coverprofile from run flags, if present.
func ExtractCoverProfile(runFlags []string) string { return extractFlag(runFlags, "-coverprofile") }

// StripCoverProfile removes -coverprofile and its value from run flags.
func StripCoverProfile(runFlags []string) []string { return stripFlag(runFlags, "-coverprofile") }

// StripNonLoadFlags removes the build flags only `go test -c` understands
// (-cover, -covermode, -coverpkg, -vet), which packages.Load rejects when
// passed as BuildFlags, and -exec, which belongs to the suite command line.
func StripNonLoadFlags(flags []string) []string {
	_, flags = ExtractExecPrefix(flags)
	return stripCompileOnlyFlags(flags)
}

func stripCompileOnlyFlags(flags []string) []string {
	var out []string
	for i := 0; i < len(flags); i++ {
		f := flags[i]
		name, _, hasEquals := strings.Cut(f, "=")
		if name == "-cover" {
			continue
		}
		if name == "-covermode" || name == "-coverpkg" || name == "-vet" {
			if !hasEquals && i+1 < len(flags) {
				i++
			}
			continue
		}
		out = append(out, f)
	}
	return out
}

// IsGoTestFlag reports whether name (the flag name without any =value suffix)
// is a recognized go test flag. It returns whether the flag takes a value
// argument and whether it is known at all.
func IsGoTestFlag(name string) (isValue bool, known bool) {
	if side, takesValue := classifyGoFlag(name); side != notGoFlag {
		return takesValue, true
	}
	if name == "-json" || name == "-args" {
		return false, true
	}
	return false, false
}

// InjectDefaultTimeout adds -timeout=10m to runFlags, ahead of -args, if the
// user did not supply one. This matches `go test` behavior — compiled test binaries have
// no timeout by default, which can cause indefinite hangs.
func InjectDefaultTimeout(runFlags []string) []string {
	for _, f := range runFlags {
		if f == "-args" {
			break
		}
		if strings.HasPrefix(f, "-timeout") {
			return runFlags
		}
	}
	return insertBeforeArgs(runFlags, "-timeout=10m")
}

// insertBeforeArgs adds flag to runFlags ahead of the -args marker, so it is
// a run flag the binary's testing package reads and not a pass-through word.
func insertBeforeArgs(runFlags []string, flag string) []string {
	if i := slices.Index(runFlags, "-args"); i >= 0 {
		return slices.Insert(slices.Clone(runFlags), i, flag)
	}
	return append(slices.Clone(runFlags), flag)
}

// InsertBeforeArgs adds a go test flag to args ahead of the -args marker, so
// it is read as a flag and never as one of the test binary's own words.
func InsertBeforeArgs(args []string, flag string) []string { return insertBeforeArgs(args, flag) }

// HasVerboseFlag reports whether flags ask for verbose output, as go test
// reads them: the last -v before -args wins, and a value that is no boolean
// (test2json) is verbose.
func HasVerboseFlag(flags []string) bool {
	verbose := false
	for _, f := range flags {
		if f == "-args" {
			break
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(f, "-"), "-"), "=")
		if name != "v" && name != "test.v" {
			continue
		}
		if !hasValue {
			verbose = true
			continue
		}
		b, err := strconv.ParseBool(value)
		verbose = b || err != nil
	}
	return verbose
}

// ExtractParallelValue returns the integer value of -parallel from run flags.
// Returns 0 if not present or not parseable. Stops scanning at -args.
func ExtractParallelValue(runFlags []string) int {
	v, _ := strconv.Atoi(extractFlag(runFlags, "-parallel"))
	return v
}

// ExtractCountValue returns how many times -count asks each benchmark or
// test to run: its value when given and parseable, otherwise 1. Stops
// scanning at -args.
func ExtractCountValue(flags []string) int {
	if v, err := strconv.Atoi(extractFlag(flags, "-count", "-test.count")); err == nil && v > 0 {
		return v
	}
	return 1
}

// InjectParallel adds -parallel=n to runFlags, ahead of -args, if not
// already present before the -args boundary.
func InjectParallel(runFlags []string, n int) []string {
	for _, f := range runFlags {
		if f == "-args" {
			break
		}
		if strings.HasPrefix(f, "-parallel") {
			return runFlags
		}
	}
	return insertBeforeArgs(runFlags, fmt.Sprintf("-parallel=%d", n))
}

// PatternIndices returns the indices of the package patterns in args, read
// as go test reads them: every positional argument that is not a flag's
// value, up to -args. own, if set, names the flags other than go test's
// that args may carry, reporting whether each takes a value; a flag neither
// knows takes the non-flag after it as its value, as go test assumes.
func PatternIndices(args []string, own func(name string) (isValue, known bool)) []int {
	var idx []int
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-args" || arg == "--args":
			return idx
		case arg == "--":
			continue
		case !strings.HasPrefix(arg, "-"):
			idx = append(idx, i)
			continue
		}
		name, _, hasEquals := strings.Cut(arg, "=")
		if hasEquals {
			continue
		}
		isValue, known := false, false
		if own != nil {
			isValue, known = own(name)
		}
		if !known {
			isValue, known = IsGoTestFlag("-" + strings.TrimLeft(name, "-"))
		}
		if !known {
			isValue = i+1 < len(args) && !strings.HasPrefix(args[i+1], "-")
		}
		if isValue {
			i++
		}
	}
	return idx
}

// ExtractProcessCap reads go test's -p from the build flags: the number of
// programs it would run at once, which caps gotest's suite processes and its
// compile parallelism alike. 0 means the flag is absent or not a number.
func ExtractProcessCap(buildFlags []string) int {
	n, _ := strconv.Atoi(extractFlag(buildFlags, "-p"))
	return max(0, n)
}

// ExtractExecPrefix lifts go test's -exec out of the build flags: the command
// line, split on whitespace as go test splits it, that every suite binary
// runs under. rest is buildFlags without it.
func ExtractExecPrefix(buildFlags []string) (prefix, rest []string) {
	for i := 0; i < len(buildFlags); i++ {
		f := buildFlags[i]
		if v, ok := strings.CutPrefix(f, "-exec="); ok {
			prefix = strings.Fields(v)
			continue
		}
		if f == "-exec" && i+1 < len(buildFlags) {
			prefix = strings.Fields(buildFlags[i+1])
			i++
			continue
		}
		rest = append(rest, f)
	}
	return prefix, rest
}
