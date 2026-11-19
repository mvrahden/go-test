package conventions_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoFlagsTestSuite holds the go test flags gotest takes to the toolchain
// in use: the build flags to `go help build`.
type GoFlagsTestSuite struct{}

func (s *GoFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// helpLine is a flag line of go help: one tab, a dash and a name; anything
// after the name means the flag takes a value. Continuation lines start
// with two tabs and never match.
var helpLine = regexp.MustCompile(`^\t-([A-Za-z0-9]+)( .*)?$`)

// helpFlags reads the flags `go help <topic>` lists, by name with its dash,
// each with whether it takes a value.
func helpFlags(t *gotest.T, topic string) map[string]bool {
	cmd := exec.CommandContext(t.Context(), "go", "help", topic)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	gotest.NoError(t, err, "go help %s: %s", topic, stderr.String())
	flags := map[string]bool{}
	for line := range strings.SplitSeq(string(out), "\n") {
		if m := helpLine.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			flags["-"+m[1]] = m[2] != ""
		}
	}
	gotest.NotEmpty(t, flags, "go help %s lists no flag this suite can read", topic)
	return flags
}

// tableLiteral reads the keys of the map literal file (relative to the
// repository root) assigns to name; for a map[string]bool also each
// key's value.
func tableLiteral(t *gotest.T, name, file string) map[string]bool {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file)), nil, 0)
	gotest.NoError(t, err)
	var lit *ast.CompositeLit
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || lit != nil {
			return lit == nil
		}
		for i, id := range spec.Names {
			if id.Name != name || i >= len(spec.Values) {
				continue
			}
			if cl, ok := spec.Values[i].(*ast.CompositeLit); ok {
				lit = cl
			}
		}
		return false
	})
	gotest.NotZero(t, lit, "%s no longer declares %s: this suite reads it by name", file, name)
	table := map[string]bool{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		gotest.True(t, ok, "%s in %s is not a map literal", name, file)
		key, ok := kv.Key.(*ast.BasicLit)
		gotest.True(t, ok, "a key of %s in %s is not a string literal", name, file)
		k, err := strconv.Unquote(key.Value)
		gotest.NoError(t, err)
		v, _ := kv.Value.(*ast.Ident)
		table[k] = v != nil && v.Name == "true"
	}
	return table
}

func hasKey[V any](m map[string]V, key string) bool {
	_, ok := m[key]
	return ok
}

const argsFile = "internal/gotestrunner/args.go"

func (s *GoFlagsTestSuite) TestBuildFlagsAreTheToolchains(t *gotest.T) {
	help := helpFlags(t, "build")
	taken, refused := tableLiteral(t, "buildFlags", argsFile), tableLiteral(t, "buildFlagsNotTaken", argsFile)

	var unlisted, gone, arity []string
	for name, value := range help {
		switch v, ok := taken[name]; {
		case ok && v != value:
			arity = append(arity, name)
		case !ok && !hasKey(refused, name):
			unlisted = append(unlisted, name)
		}
	}
	for name := range taken {
		if !hasKey(help, name) {
			gone = append(gone, name)
		}
	}
	for name := range refused {
		if !hasKey(help, name) || hasKey(taken, name) {
			gone = append(gone, name)
		}
	}
	slices.Sort(unlisted)
	slices.Sort(gone)
	slices.Sort(arity)
	gotest.Empty(t, unlisted, "go help build lists these and internal/gotestrunner/args.go does not: add each to buildFlags, or to buildFlagsNotTaken with the reason")
	gotest.Empty(t, gone, "internal/gotestrunner/args.go lists these and go help build of %s does not: remove them", runtime.Version())
	gotest.Empty(t, arity, "these take a value in one of go help build and buildFlags and not in the other")
}
