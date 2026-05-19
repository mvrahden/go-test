package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// Invocation carries the resolved state for a single CLI invocation.
type Invocation struct {
	Args   []string
	Config config.ProjectConfig
}

// TagArgs returns args with -tags prepended from config, if not already set.
func (inv Invocation) TagArgs() []string {
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
func (inv Invocation) DefaultArgs() []string {
	out := inv.TagArgs()
	if inv.Config.SetupTimeout.Duration() > 0 && !hasFlag(out, "--setup-timeout") {
		out = append([]string{"--setup-timeout=" + inv.Config.SetupTimeout.Duration().String()}, out...)
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
	Debug           bool
	CI              bool
	JSON            bool
	UpdateSnapshots bool
}

// knownSubcommands is the set of recognized subcommands.
var knownSubcommands = map[string]bool{
	"discover": true,
	"prepare":  true,
	"generate": true,
	"scaffold": true,
	"migrate":  true,
	"spec":     true,
	"watch":    true,
	"clean":    true,
	"lint":     true,
	"refactor": true,
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
// gotest flags use --double-dash; go test flags use -single-dash.
// Both domains are validated against their respective registries.
// A bare "--" acts as an escape hatch: everything after it goes to
// goTestArgs without validation.
func SplitArgs(args []string, allowed map[string]bool) (ownArgs, goTestArgs []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			goTestArgs = append(goTestArgs, args[i+1:]...)
			return
		}

		if arg == "-args" {
			goTestArgs = append(goTestArgs, args[i:]...)
			return
		}

		if !strings.HasPrefix(arg, "-") {
			goTestArgs = append(goTestArgs, arg)
			continue
		}

		name, _, hasEquals := strings.Cut(arg, "=")

		if strings.HasPrefix(name, "--") {
			kind := gotestFlags[name]
			if kind == 0 {
				return nil, nil, fmt.Errorf("unknown flag: %s", name)
			}
			if !allowed[name] {
				return nil, nil, fmt.Errorf("flag %s is not valid for this subcommand", name)
			}
			ownArgs = append(ownArgs, arg)
			if !hasEquals && kind == ValueFlag && i+1 < len(args) {
				i++
				ownArgs = append(ownArgs, args[i])
			}
			continue
		}

		isValue, known := gotestrunner.IsGoTestFlag(name)
		if !known {
			return nil, nil, fmt.Errorf("unknown flag: %s", name)
		}
		goTestArgs = append(goTestArgs, arg)
		if !hasEquals && isValue && i+1 < len(args) {
			i++
			goTestArgs = append(goTestArgs, args[i])
		}
	}
	return
}

func extractStringFlag(args []string, name, defaultVal string) string {
	for i, arg := range args {
		if v, ok := strings.CutPrefix(arg, name+"="); ok {
			return v
		}
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return defaultVal
}

func ExtractPackagePatterns(goTestArgs []string) []string {
	var patterns []string
	for _, arg := range goTestArgs {
		if arg == "-args" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if gotestrunner.LooksLikePackagePattern(arg) {
			patterns = append(patterns, arg)
		}
	}
	if len(patterns) == 0 {
		return []string{"."}
	}
	return patterns
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
