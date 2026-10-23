package gotestrunner

import (
	"flag"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// testFlags lists the test binary's flags as the testing package defines
// them, by name without the -test. prefix and value type.
var testFlags = sync.OnceValue(func() map[string]reflect.Type {
	testing.Init()
	types := map[string]reflect.Type{}
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		if name, ok := strings.CutPrefix(f.Name, "test."); ok {
			if t := reflect.TypeOf(f.Value); t.Kind() == reflect.Pointer {
				types[name] = t.Elem()
			}
		}
	})
	return types
})

// testFlagSet holds those flags on fresh values of their own types: the
// process's own flags stay untouched, and each caller parses its own.
func testFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("go test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for name, t := range testFlags() {
		if v, ok := reflect.New(t).Interface().(flag.Value); ok {
			fs.Var(v, name, "")
		}
	}
	return fs
}

// ValidateTestFlags checks the values of the test binary's flags in args
// before anything is built, as go test does: a bad value is a usage error,
// not a failing test. -run, -skip, -bench and -fuzz must be patterns the
// testing package compiles, split as it splits them. A repeated flag is
// judged by its last value, the one the binary keeps.
func ValidateTestFlags(args []string) error {
	fs := testFlagSet()
	var known []string
	last := map[string]string{}
	var order []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-args" || arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, val, hasEquals := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		f := fs.Lookup(name)
		if f == nil {
			continue
		}
		isBool := isBoolFlag(f)
		if !hasEquals && !isBool && i+1 < len(args) {
			i++
			val, hasEquals = args[i], true
		}
		if !hasEquals && !isBool {
			return fmt.Errorf("flag needs an argument: -%s", name)
		}
		if hasEquals {
			known = append(known, "-"+name+"="+val)
		} else {
			known = append(known, "-"+name)
		}
		if _, seen := last[name]; !seen {
			order = append(order, name)
		}
		last[name] = val
	}
	if err := fs.Parse(known); err != nil {
		return err
	}
	// The testing package reads these strings only once tests run.
	for _, name := range order {
		val := last[name]
		switch name {
		case "run", "skip", "bench", "fuzz":
			if val == "" {
				continue
			}
			if _, err := parseNameFilter(val, "-"+name); err != nil {
				return fmt.Errorf("invalid regexp for %v", err)
			}
		case "shuffle":
			if _, err := strconv.ParseInt(val, 10, 64); err != nil && val != "on" && val != "off" {
				return fmt.Errorf("invalid value %q for flag -shuffle: must be off, on or an integer", val)
			}
		case "cpu":
			for _, n := range strings.Split(val, ",") {
				if c, err := strconv.Atoi(n); err != nil || c <= 0 {
					return fmt.Errorf("invalid value %q for flag -cpu: must be a list of positive integers", val)
				}
			}
		}
	}
	return nil
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}
