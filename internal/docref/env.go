package docref

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// EnvVar is an environment variable a user may set.
type EnvVar struct{ Name, Doc string }

// UserEnv are the variables a user may set, in help's order, then the ones
// help does not name.
var UserEnv = []EnvVar{
	{"GOTEST_CI", "CI mode on for any value but `0`, `f`, `F`, `false`, `FALSE` and `False`, which turn it off; unset, the standard `CI` variable turns it on"},
	{"GOTEST_CACHE_DIR", "Cache location for test binaries (default `os.UserCacheDir()/gotest`; a run evicts the binaries of a gone checkout, those unused for 7 days, then the least recently used above 5 GiB)"},
	{"GOTEST_SLOW_BUILD_THRESHOLD", "How long a build runs, as a Go duration, before the CLI says it is still building (default `15s`; unparsable or not positive → the default)"},
	{"GOTEST_UPDATE_SNAPSHOTS", "Regenerate snapshot baselines, off for the same values as `GOTEST_CI` (what `--update-snapshots` sets; the only mechanism under plain `go test`)"},
	{"GOTEST_TIMEOUT_SCALE", "Multiply every suite and fixture budget, declared or default, deadline and verdict alike, for a machine slower than the one the budgets were written on (`2` doubles them); the processes the tests start do not inherit it"},
}

// internalEnv are the variables gotest sets for its own processes.
var internalEnv = []string{
	"GOTEST_SHARED_STATE_FILE",
	"GOTEST_SHARED_PROTOCOL_FD",
	"GOTEST_FUZZ_ECHO_INPUT",
	"GOTEST_CONTROL",
	"GOTEST_INTERNAL_INTERRUPT_CONSOLE",
	"GOTEST_STOP_FILE",
}

// EnvSections returns the renderer of env/user.
func EnvSections() map[string]Renderer {
	return map[string]Renderer{"env/user": formats(map[string]func() string{
		"table": func() string {
			var b strings.Builder
			b.WriteString("| Variable | Effect |\n|---|---|\n")
			for _, v := range UserEnv {
				fmt.Fprintf(&b, "| `%s` | %s |\n", v.Name, textCell(v.Doc))
			}
			return b.String()
		},
		"html": func() string {
			var b strings.Builder
			for _, v := range UserEnv {
				fmt.Fprintf(&b, "<tr><td><code>%s</code></td><td>%s.</td></tr>\n", v.Name, htmlText(v.Doc))
			}
			return b.String()
		},
	})}
}

var envName = regexp.MustCompile(`^GOTEST_[A-Z0-9_]+$`)

// testPlumbing are prefixes of variables only gotest's own tests set.
var testPlumbing = []string{"GOTEST_TEST_", "GOTEST_MP_", "GOTEST_PROCTREE_", "GOTEST_CONTROL_TEST_"}

// CheckEnv compares UserEnv and internalEnv with every "GOTEST_…" string
// literal in the non-test Go files under the dirs. Test data, the testkit
// and the drill are test plumbing and not read.
func CheckEnv(dirs ...string) error {
	seen := map[string]string{} // name → first file:line naming it
	for _, dir := range dirs {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(path)
			if d.IsDir() {
				if d.Name() == "testdata" || strings.HasSuffix(slash, "/internal/testkit") || strings.HasSuffix(slash, "/internal/drill") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			return envLiterals(path, seen)
		}); err != nil {
			return err
		}
	}
	known := map[string]bool{}
	for _, v := range UserEnv {
		known[v.Name] = true
	}
	for _, name := range internalEnv {
		known[name] = true
	}
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(seen)) {
		if !known[name] {
			errs = append(errs, fmt.Errorf("%s is read by %s and is in neither UserEnv nor internalEnv of internal/docref; add it to one", name, seen[name]))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(known)) {
		if _, ok := seen[name]; !ok {
			errs = append(errs, fmt.Errorf("%s is listed in internal/docref and no code names it; remove it", name))
		}
	}
	return errors.Join(errs...)
}

// envLiterals records the GOTEST_ string literals of one file.
func envLiterals(path string, seen map[string]string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil || !envName.MatchString(name) || slices.ContainsFunc(testPlumbing, func(p string) bool { return strings.HasPrefix(name, p) }) {
			return true
		}
		if _, ok := seen[name]; !ok {
			pos := fset.Position(lit.Pos())
			seen[name] = fmt.Sprintf("%s:%d", filepath.ToSlash(pos.Filename), pos.Line)
		}
		return true
	})
	return nil
}
