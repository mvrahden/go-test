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
	"-race":     true,
	"-msan":     true,
	"-asan":     true,
	"-cover":    true,
	"-a":        true,
	"-c":        true,
	"-trimpath": true,
	"-work":     true,
}

var buildSpecialValueFlags = map[string]bool{
	"-o":    true,
	"-exec": true,
}

var buildValueFlags = map[string]bool{
	"-covermode": true,
	"-coverpkg":  true,
	"-tags":      true,
	"-ldflags":   true,
	"-gcflags":   true,
	"-asmflags":  true,
	"-mod":       true,
	"-modfile":   true,
	"-p":         true,
	"-toolexec":  true,
	"-pkgdir":    true,
}

// runOnlyFlags are flags consumed exclusively by the test binary at runtime.
var runOnlyFlags = map[string]bool{
	"-v":         true,
	"-failfast":  true,
	"-short":     true,
	"-benchmem":  true,
	"-fullpath":  true,
}

var runValueFlags = map[string]bool{
	"-count":             true,
	"-timeout":           true,
	"-run":               true,
	"-parallel":          true,
	"-shuffle":           true,
	"-bench":             true,
	"-benchtime":         true,
	"-coverprofile":      true,
	"-cpuprofile":        true,
	"-memprofile":        true,
	"-blockprofile":      true,
	"-mutexprofile":      true,
	"-trace":             true,
	"-outputdir":         true,
	"-list":              true,
	"-fuzz":              true,
	"-fuzztime":          true,
	"-fuzzminimizetime":  true,
	"-cpu":               true,
	"-blockprofilerate":  true,
	"-memprofilerate":    true,
	"-mutexprofilerate":  true,
}

// ClassifiedArgs holds the result of splitting go test arguments.
type ClassifiedArgs struct {
	BuildFlags  []string // flags for `go test -c`
	RunFlags    []string // flags for the test binary (without -test. prefix)
	PkgPatterns []string // package patterns like ./...
}

// ClassifyGoTestArgs splits go test arguments into build flags, run flags,
// and package patterns. Flags after -args are treated as pass-through and
// appended to RunFlags.
func ClassifyGoTestArgs(args []string) ClassifiedArgs {
	var result ClassifiedArgs
	needsCoverBuild := false

	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "-args" {
			result.RunFlags = append(result.RunFlags, args[i:]...)
			break
		}

		if !strings.HasPrefix(arg, "-") {
			if LooksLikePackagePattern(arg) {
				result.PkgPatterns = append(result.PkgPatterns, arg)
			}
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
			if hasEquals {
				result.BuildFlags = append(result.BuildFlags, arg)
				i++
			} else if i+1 < len(args) {
				result.BuildFlags = append(result.BuildFlags, arg, args[i+1])
				i += 2
			} else {
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
			if hasEquals {
				result.RunFlags = append(result.RunFlags, arg)
				i++
			} else if i+1 < len(args) {
				result.RunFlags = append(result.RunFlags, arg, args[i+1])
				i += 2
			} else {
				result.RunFlags = append(result.RunFlags, arg)
				i++
			}
			continue
		}

		// Unrecognized flag: pass to build (conservative — unknown flags
		// are more likely build-related custom tooling than test binary flags).
		if hasEquals {
			result.BuildFlags = append(result.BuildFlags, arg)
			i++
		} else {
			result.BuildFlags = append(result.BuildFlags, arg)
			i++
		}
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
// by adding the -test. prefix. Flags after -args are left untouched.
func TranslateToTestBinaryFlags(flags []string) []string {
	out := make([]string, 0, len(flags))
	passthrough := false
	for _, f := range flags {
		if f == "-args" {
			passthrough = true
			out = append(out, f)
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

// ExtractRunFilter returns the value of -run from run flags, if present.
func ExtractRunFilter(runFlags []string) string {
	for i, f := range runFlags {
		if f == "-args" {
			return ""
		}
		if v, ok := strings.CutPrefix(f, "-run="); ok {
			return v
		}
		if f == "-run" && i+1 < len(runFlags) {
			return runFlags[i+1]
		}
	}
	return ""
}

// StripRunFilter removes -run and its value from run flags.
func StripRunFilter(runFlags []string) []string {
	var out []string
	for i := 0; i < len(runFlags); i++ {
		f := runFlags[i]
		if f == "-args" {
			out = append(out, runFlags[i:]...)
			return out
		}
		if strings.HasPrefix(f, "-run=") {
			continue
		}
		if f == "-run" {
			i++ // skip value
			continue
		}
		out = append(out, f)
	}
	return out
}

// ExtractCoverProfile returns the value of -coverprofile from run flags, if present.
func ExtractCoverProfile(runFlags []string) string {
	for i, f := range runFlags {
		if f == "-args" {
			return ""
		}
		if v, ok := strings.CutPrefix(f, "-coverprofile="); ok {
			return v
		}
		if f == "-coverprofile" && i+1 < len(runFlags) {
			return runFlags[i+1]
		}
	}
	return ""
}

// StripCoverProfile removes -coverprofile and its value from run flags.
func StripCoverProfile(runFlags []string) []string {
	var out []string
	for i := 0; i < len(runFlags); i++ {
		f := runFlags[i]
		if f == "-args" {
			out = append(out, runFlags[i:]...)
			return out
		}
		if strings.HasPrefix(f, "-coverprofile=") {
			continue
		}
		if f == "-coverprofile" {
			i++ // skip value
			continue
		}
		out = append(out, f)
	}
	return out
}

// StripCoverBuildFlags removes coverage-related build flags (-cover,
// -covermode, -coverpkg) that break packages.Load when passed as BuildFlags.
func StripCoverBuildFlags(flags []string) []string {
	var out []string
	for i := 0; i < len(flags); i++ {
		f := flags[i]
		name, _, hasEquals := strings.Cut(f, "=")
		if name == "-cover" {
			continue
		}
		if name == "-covermode" || name == "-coverpkg" {
			if !hasEquals && i+1 < len(flags) {
				i++
			}
			continue
		}
		out = append(out, f)
	}
	return out
}

// InjectChecklinkname ensures -ldflags includes -checklinkname=0 so that
// linkname references in generated overlay code can link correctly
// (required for Go 1.23+ which restricts linkname to stdlib symbols).
func InjectChecklinkname(buildFlags []string) []string {
	const ldflag = "-checklinkname=0"
	buildFlags = slices.Clone(buildFlags)
	for i, f := range buildFlags {
		if strings.HasPrefix(f, "-ldflags=") {
			buildFlags[i] = f + " " + ldflag
			return buildFlags
		}
		if f == "-ldflags" && i+1 < len(buildFlags) {
			buildFlags[i+1] = buildFlags[i+1] + " " + ldflag
			return buildFlags
		}
	}
	return append(buildFlags, "-ldflags="+ldflag)
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

// InjectDefaultTimeout adds -timeout=10m to runFlags if the user did not
// supply one. This matches `go test` behavior — compiled test binaries have
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
	return append(runFlags, "-timeout=10m")
}

func HasVerboseFlag(flags []string) bool {
	return slices.ContainsFunc(flags, func(f string) bool {
		return f == "-v" || f == "-v=true"
	})
}

// ExtractParallelValue returns the integer value of -parallel from run flags.
// Returns 0 if not present or not parseable. Stops scanning at -args.
func ExtractParallelValue(runFlags []string) int {
	for i, f := range runFlags {
		if f == "-args" {
			return 0
		}
		if v, ok := strings.CutPrefix(f, "-parallel="); ok {
			n, _ := strconv.Atoi(v)
			return n
		}
		if f == "-parallel" && i+1 < len(runFlags) {
			n, _ := strconv.Atoi(runFlags[i+1])
			return n
		}
	}
	return 0
}

// InjectParallel appends -parallel=n to runFlags if not already present
// before the -args boundary.
func InjectParallel(runFlags []string, n int) []string {
	for _, f := range runFlags {
		if f == "-args" {
			break
		}
		if strings.HasPrefix(f, "-parallel") {
			return runFlags
		}
	}
	return append(runFlags, fmt.Sprintf("-parallel=%d", n))
}

func LooksLikePackagePattern(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.Contains(s, "/") || strings.Contains(s, "\\")
}
