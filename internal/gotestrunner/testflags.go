package gotestrunner

import (
	"flag"
	"fmt"
	"io"
	"reflect"
	"regexp"
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
// not a failing test. -run and -skip must be regular expressions.
func ValidateTestFlags(args []string) error {
	fs := testFlagSet()
	var known []string
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
		// The testing package reads these strings only once tests run.
		switch name {
		case "run", "skip":
			if _, err := regexp.Compile(val); err != nil {
				return fmt.Errorf("invalid regexp for -%s: %v", name, err)
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
	return fs.Parse(known)
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}
