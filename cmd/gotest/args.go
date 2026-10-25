package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/pkgpattern"
)

// Invocation carries the resolved state for a single CLI invocation.
type Invocation struct {
	Args   []string
	Config config.ProjectConfig
}

// TagArgs returns args with -tags prepended from config, if not already set.
func (inv Invocation) TagArgs() []string { //nolint:gocritic // hugeParam: stable API
	if inv.Config.Tags == "" {
		return inv.Args
	}
	if hasFlag(inv.Args, "-tags") {
		return inv.Args
	}
	return append([]string{"-tags=" + inv.Config.Tags}, inv.Args...)
}

// DefaultArgs returns args with tags and setup-timeout from config,
// each only if not already set via CLI flags.
func (inv Invocation) DefaultArgs() []string { //nolint:gocritic // hugeParam: stable API
	out := inv.TagArgs()
	if inv.Config.SetupTimeout != nil && !hasFlag(out, "--setup-timeout") {
		out = append([]string{"--setup-timeout=" + inv.Config.SetupTimeout.Duration().String()}, out...)
	}
	if inv.Config.Timeout != nil && !hasFlag(out, "--timeout") {
		out = append([]string{"--timeout=" + inv.Config.Timeout.Duration().String()}, out...)
	}
	return out
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

type ExecConfig struct {
	GoTestArgs      []string
	PackagePatterns []string
	SetupTimeout    time.Duration
	GlobalTimeout   time.Duration
	Debug           bool
	CI              bool
	JSON            bool
	UpdateSnapshots bool
	NoCache         bool
	Parallel        int
	CompileParallel int
	HarvestSeeds    bool
	// Events receives the run's go test -json event stream as it is
	// written; nil without --events-file.
	Events io.Writer
}

// knownSubcommands is the set of recognized subcommands.
var knownSubcommands = map[string]bool{
	"discover": true,
	"prepare":  true,
	"bench":    true,
	"fuzz":     true,
	"generate": true,
	"scaffold": true,
	"migrate":  true,
	"spec":     true,
	"summary":  true,
	"watch":    true,
	"clean":    true,
	"lint":     true,
	"refactor": true,
	"skill":    true,
	"version":  true,
	"help":     true,
}

// ParseSubcommand checks the first positional argument against known
// subcommands. If it matches, it is consumed and returned along with
// the remaining args. Otherwise, subcmd is empty and the full args
// slice is returned unchanged.
func ParseSubcommand(args []string) (subcmd string, remaining []string) {
	if len(args) == 0 {
		return "", nil
	}
	first := args[0]
	if knownSubcommands[first] {
		remaining = args[1:]
		if len(remaining) == 0 {
			remaining = nil
		}
		return first, remaining
	}
	return "", args
}

// SplitArgs classifies args into gotest flags and go test flags.
//
// gotest flags use --double-dash and are returned parsed: a boolean takes
// --name, --name=true or --name=false; a value flag takes --name=value or
// --name value, and one without a value is a usage error; the last occurrence
// wins. go test flags use -single-dash and are validated against their
// registry. A bare "--" is an escape hatch: everything after it goes to
// goTestArgs without validation, as does everything from "-args" on.
func SplitArgs(args []string, allowed map[string]bool) (own OwnFlags, goTestArgs []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			goTestArgs = append(goTestArgs, args[i+1:]...)
			return own, goTestArgs, nil
		}

		if arg == "-args" {
			goTestArgs = append(goTestArgs, args[i:]...)
			return own, goTestArgs, nil
		}

		if !strings.HasPrefix(arg, "-") {
			goTestArgs = append(goTestArgs, arg)
			continue
		}

		name, value, hasEquals := strings.Cut(arg, "=")

		if strings.HasPrefix(name, "--") {
			kind := gotestFlags[name]
			if kind == 0 {
				return OwnFlags{}, nil, fmt.Errorf("unknown flag: %s", name)
			}
			if !allowed[name] {
				return OwnFlags{}, nil, fmt.Errorf("flag %s is not valid for this subcommand", name)
			}
			switch kind {
			case BoolFlag:
				if !hasEquals {
					own.set(name, "true")
					continue
				}
				b, perr := strconv.ParseBool(value)
				if perr != nil {
					return OwnFlags{}, nil, fmt.Errorf("invalid boolean value %q for %s", value, name)
				}
				own.set(name, strconv.FormatBool(b))
			case ValueFlag:
				if hasEquals {
					own.set(name, value)
					continue
				}
				if i+1 >= len(args) {
					return OwnFlags{}, nil, fmt.Errorf("flag %s needs a value", name)
				}
				i++
				own.set(name, args[i])
			}
			continue
		}

		isValue, known := gotestrunner.IsGoTestFlag(name)
		if !known {
			return OwnFlags{}, nil, fmt.Errorf("unknown flag: %s", name)
		}
		goTestArgs = append(goTestArgs, arg)
		if !hasEquals && isValue && i+1 < len(args) {
			i++
			goTestArgs = append(goTestArgs, args[i])
		}
	}
	if err := gotestrunner.ValidateTestFlags(goTestArgs); err != nil {
		return OwnFlags{}, nil, err
	}
	return own, goTestArgs, nil
}

// ExtractPackagePatterns returns the package patterns in args as go test
// reads them: every positional argument that is not a flag's value, meta
// patterns (all, std, tool, work) and bare import paths (fmt) included.
// Without one, or for an empty one, it is the current directory. Go files
// are a usage error: go test builds them as an ad-hoc package, which has no
// suites to generate for.
func ExtractPackagePatterns(args []string) ([]string, error) {
	var patterns []string
	for _, i := range gotestrunner.PatternIndices(args, ownFlag) {
		patterns = append(patterns, args[i])
	}
	if len(patterns) == 0 {
		return []string{"."}, nil
	}
	if files := pkgpattern.Files(patterns); len(files) > 0 {
		return nil, fmt.Errorf("gotest takes packages, not files: %s", strings.Join(files, ", "))
	}
	return pkgpattern.Clean(patterns), nil
}

// ownFlag reports whether a gotest flag takes a value.
func ownFlag(name string) (isValue, known bool) {
	kind, ok := gotestFlags[name]
	return kind == ValueFlag, ok
}

func extractTagsFlag(args []string) (tags string, remaining []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if v, ok := strings.CutPrefix(arg, "-tags="); ok {
			tags = v
		} else if arg == "-tags" && i+1 < len(args) {
			tags = args[i+1]
			i++
		} else {
			remaining = append(remaining, arg)
		}
	}
	return tags, remaining
}
