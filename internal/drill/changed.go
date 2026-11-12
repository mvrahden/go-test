package drill

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// MutantFacts is what a mutant's patch and judge runs say a change can touch.
type MutantFacts struct {
	Targets  []string // files the diff patches
	Clauses  []string // the THEN text of each Clause: line
	Packages []string // judge package directories
}

// ReadFacts reads a patch's header and diff, and its judge runs.
func ReadFacts(patch string, runs []Run) MutantFacts {
	var f MutantFacts
	header := true
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "--- ") {
			header = false
		}
		if header {
			if rest, ok := strings.CutPrefix(line, "Clause:"); ok {
				if _, then, found := strings.Cut(rest, " → "); found {
					f.Clauses = appendNew(f.Clauses, strings.TrimSpace(then))
				}
			}
			continue
		}
		for _, prefix := range []string{"--- a/", "+++ b/"} {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				path, _, _ := strings.Cut(rest, "\t")
				f.Targets = appendNew(f.Targets, path)
			}
		}
	}
	for _, r := range runs {
		for _, p := range r.Packages {
			p = strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/...")
			f.Packages = appendNew(f.Packages, p)
		}
	}
	return f
}

func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// ChangedFiles reads DRILL_CHANGED: the listed paths, and whether it is set.
// named says mutants were given on the command line, which it refuses.
func ChangedFiles(environ map[string]string, named bool) (changed []string, set bool, err error) {
	path, set := environ["DRILL_CHANGED"]
	if !set {
		return nil, false, nil
	}
	if named {
		return nil, true, errors.New("DRILL_CHANGED selects the mutants itself: name mutants or set it, not both")
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: the caller names the list
	if err != nil {
		return nil, true, errors.New("DRILL_CHANGED names a file that cannot be read: " + path)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimPrefix(strings.TrimSpace(line), "./"); line != "" {
			changed = append(changed, line)
		}
	}
	return changed, true, nil
}

// everything names the paths a change to which selects every mutant: the
// module's requirements, the gate's settings, the driver, the planner and
// the kit nearly every judge package runs the CLI with.
func everything(path string) bool {
	switch path {
	case "go.mod", "go.sum", "go.work", "go.work.sum", ".github/gate.env", "tests/drill/drill.sh":
		return true
	}
	for _, dir := range []string{"internal/drill/", "tests/drill/driver/", "internal/speccatalog/", "internal/testkit/"} {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	return false
}

// Narrow returns the names a change to changed can touch, in names' order,
// and the line that says so. spec returns a spec file's text, "" when absent.
func Narrow(names, changed []string, facts map[string]MutantFacts, spec func(path string) string) (selected []string, note string) {
	for _, path := range changed {
		if everything(path) {
			return slices.Clone(names), "drill  " + path + " changed: every mutant is selected"
		}
	}
	specs := map[string]string{}
	for _, path := range changed {
		if strings.HasPrefix(path, "specs/") && strings.HasSuffix(path, "/spec.md") {
			specs[path] = spec(path)
		}
	}
	selected = []string{}
	for _, name := range names {
		if touches(name, changed, facts[name], specs) {
			selected = append(selected, name)
		}
	}
	return selected, fmt.Sprintf("drill  the changed files select %d of %d mutants", len(selected), len(names))
}

// sharedFixtures are fixture trees judge packages read from outside their
// own directory, and the judge packages that read them. The conformance
// suites name their fixtures through variables as often as literals, so a
// change to one counts for every conformance package.
var sharedFixtures = []struct{ tree, judges string }{
	{"tests/conformance/testdata/", "tests/conformance/"},
	{"tests/canary/testdata/broken/", "tests/conformance/cli/"},
}

func touches(name string, changed []string, f MutantFacts, specs map[string]string) bool {
	for _, path := range changed {
		if path == "tests/drill/mutants/"+name+".patch" || slices.Contains(f.Targets, path) {
			return true
		}
		for _, shared := range sharedFixtures {
			if !strings.HasPrefix(path, shared.tree) {
				continue
			}
			for _, pkg := range f.Packages {
				if strings.HasPrefix(pkg+"/", shared.judges) {
					return true
				}
			}
		}
		for _, pkg := range f.Packages {
			// A run over the whole module judges with every file.
			if pkg == "." || pkg == "..." || strings.HasPrefix(path, pkg+"/") {
				return true
			}
		}
		if text, ok := specs[path]; ok {
			for _, clause := range f.Clauses {
				if strings.Contains(text, clause) {
					return true
				}
			}
		}
	}
	return false
}
