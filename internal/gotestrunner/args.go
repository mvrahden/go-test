package gotestrunner

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// buildOnlyFlags are flags consumed exclusively by `go test -c` (compilation).
// Boolean flags have empty string value; value flags list their name only.
var buildOnlyFlags = map[string]bool{
	"-race":       true,
	"-msan":       true,
	"-asan":       true,
	"-cover":      true,
	"-a":          true,
	"-c":          true,
	"-n":          true,
	"-x":          true,
	"-trimpath":   true,
	"-work":       true,
	"-linkshared": true,
	"-modcacherw": true,
	// -buildvcs takes an optional value (-buildvcs, -buildvcs=false), so a
	// bare one must not swallow the next word.
	"-buildvcs": true,
}

var buildSpecialValueFlags = map[string]bool{
	"-o":    true,
	"-exec": true,
}

var buildValueFlags = map[string]bool{
	"-covermode":     true,
	"-coverpkg":      true,
	"-vet":           true,
	"-tags":          true,
	"-ldflags":       true,
	"-gcflags":       true,
	"-gccgoflags":    true,
	"-asmflags":      true,
	"-mod":           true,
	"-modfile":       true,
	"-p":             true,
	"-toolexec":      true,
	"-pkgdir":        true,
	"-pgo":           true,
	"-overlay":       true,
	"-buildmode":     true,
	"-compiler":      true,
	"-installsuffix": true,
}

// runOnlyFlags are flags consumed exclusively by the test binary at runtime.
var runOnlyFlags = map[string]bool{
	"-v":        true,
	"-failfast": true,
	"-short":    true,
	"-benchmem": true,
	"-fullpath": true,
}

var runValueFlags = map[string]bool{
	"-count":            true,
	"-timeout":          true,
	"-run":              true,
	"-skip":             true,
	"-parallel":         true,
	"-shuffle":          true,
	"-bench":            true,
	"-benchtime":        true,
	"-coverprofile":     true,
	"-cpuprofile":       true,
	"-memprofile":       true,
	"-blockprofile":     true,
	"-mutexprofile":     true,
	"-trace":            true,
	"-outputdir":        true,
	"-list":             true,
	"-fuzz":             true,
	"-fuzztime":         true,
	"-fuzzminimizetime": true,
	"-cpu":              true,
	"-blockprofilerate": true,
	"-memprofilerate":   true,
	"-mutexprofilerate": true,
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

		if buildOnlyFlags[name] || buildOnlyFlags[arg] {
			result.BuildFlags = append(result.BuildFlags, arg)
			i++
			continue
		}

		if buildValueFlags[name] || buildSpecialValueFlags[name] {
			switch {
			case hasEquals:
				result.BuildFlags = append(result.BuildFlags, arg)
				i++
			case i+1 < len(args):
				result.BuildFlags = append(result.BuildFlags, arg, args[i+1])
				i += 2
			default:
				result.BuildFlags = append(result.BuildFlags, arg)
				i++
			}
			continue
		}

		if runOnlyFlags[name] || runOnlyFlags[arg] {
			result.RunFlags = append(result.RunFlags, arg)
			i++
			continue
		}

		if runValueFlags[name] {
			if name == "-coverprofile" {
				needsCoverBuild = true
			}
			switch {
			case hasEquals:
				result.RunFlags = append(result.RunFlags, arg)
				i++
			case i+1 < len(args):
				result.RunFlags = append(result.RunFlags, arg, args[i+1])
				i += 2
			default:
				result.RunFlags = append(result.RunFlags, arg)
				i++
			}
			continue
		}

		// A flag nobody here declares is the test binary's. As go test
		// assumes, a non-flag after it is taken as its value.
		binaryFlags = append(binaryFlags, arg)
		i++
		if !hasEquals && i < len(args) && !strings.HasPrefix(args[i], "-") {
			binaryFlags = append(binaryFlags, args[i])
			i++
		}
	}

	if len(binaryFlags) > 0 || explicitArgs != nil {
		result.RunFlags = append(result.RunFlags, "-args")
		result.RunFlags = append(result.RunFlags, binaryFlags...)
		result.RunFlags = append(result.RunFlags, explicitArgs...)
	}

	if needsCoverBuild {
		hasCover := false
		for _, f := range result.BuildFlags {
			if f == "-cover" {
				hasCover = true
				break
			}
		}
		if !hasCover {
			result.BuildFlags = append(result.BuildFlags, "-cover")
		}
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
	if buildOnlyFlags[name] {
		return false, true
	}
	if buildValueFlags[name] || buildSpecialValueFlags[name] {
		return true, true
	}
	if runOnlyFlags[name] {
		return false, true
	}
	if runValueFlags[name] {
		return true, true
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
