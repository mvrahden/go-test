// Command gotest-openspec reports which OpenSpec scenarios have a gotest
// behavior, and how that behavior fared.
//
//	go tool gotest spec --format=json ./... | go tool gotest-openspec openspec/specs
//
// A scenario matches a behavior when its title equals an It label, or a When
// condition followed by an It label; case, punctuation and spacing are folded.
// Exit 1 when any scenario has no behavior or a failing one, so the command
// doubles as a verification gate; exit 2 when the inputs cannot be read.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

// node is the subset of a gotest spec JSON node the reconciliation reads.
type node struct {
	Display  string `json:"display"`
	Vocab    string `json:"vocab"`
	Status   string `json:"status"`
	Children []node `json:"children"`
}

type specTree struct {
	Packages []struct {
		Nodes []node `json:"nodes"`
	} `json:"packages"`
}

var (
	scenarioLine = regexp.MustCompile(`^#{3,4}\s+Scenario:\s*(.+?)\s*$`)
	nonWord      = regexp.MustCompile(`[^a-z0-9 ]+`)
)

// key folds case, punctuation and spacing so "Clamps to zero." meets "clamps to zero".
func key(s string) string {
	return strings.Join(strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

// scenarios collects every "#### Scenario:" heading under dir, in file order.
// Reads go through an os.Root so a symlink cannot lead outside dir.
func scenarios(dir string) (out []string, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		src, err := root.ReadFile(p)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(src), "\n") {
			if m := scenarioLine.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		}
		return nil
	})
	return out, err
}

// behaviors indexes every It leaf by its label and by "<condition> <label>",
// so a scenario may spell its context too. Status is the leaf's verdict.
func behaviors(tree specTree) map[string]string {
	found := map[string]string{}
	var walk func(n node, ctx string)
	walk = func(n node, ctx string) {
		switch n.Vocab {
		case "it":
			found[key(n.Display)] = n.Status
			if ctx != "" {
				found[key(ctx+" "+n.Display)] = n.Status
			}
		case "when":
			ctx = strings.TrimPrefix(n.Display, "when ")
		}
		for _, c := range n.Children {
			walk(c, ctx)
		}
	}
	for _, p := range tree.Packages {
		for _, n := range p.Nodes {
			walk(n, "")
		}
	}
	return found
}

var verdicts = map[string]string{
	"pass": "✓ verified",
	"fail": "✗ failing",
	"skip": "~ skipped",
	"none": "· declared",
}

const noBehavior = "? no behavior"

// report prints one line per scenario and a summary, returning the exit code.
func report(w io.Writer, names []string, found map[string]string) int {
	counts := map[string]int{}
	for _, name := range names {
		v := noBehavior
		if st, ok := found[key(name)]; ok {
			v = verdicts[st]
		}
		counts[v]++
		fmt.Fprintf(w, "%-14s %s\n", v, name)
	}
	fmt.Fprintf(w, "\n%d scenarios: %d verified, %d failing, %d declared, %d without a behavior\n",
		len(names), counts[verdicts["pass"]], counts[verdicts["fail"]], counts[verdicts["none"]], counts[noBehavior])
	if counts[noBehavior]+counts[verdicts["fail"]] > 0 {
		return 1
	}
	return 0
}

func run(stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: gotest spec --format=json ./... | gotest-openspec <specs-dir>")
		return 2
	}
	var tree specTree
	if err := json.NewDecoder(stdin).Decode(&tree); err != nil {
		fmt.Fprintln(stderr, "read spec json:", err)
		return 2
	}
	names, err := scenarios(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "read scenarios:", err)
		return 2
	}
	return report(stdout, names, behaviors(tree))
}

func main() {
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr, os.Args[1:]))
}
