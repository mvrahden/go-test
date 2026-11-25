package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
)

// gotestModule is the module path of gotest itself.
const gotestModule = "github.com/mvrahden/go-test"

// hookEvents maps each event gotest hook takes to the hook event name the
// agent's JSON carries for it.
var hookEvents = map[string]string{
	"post-edit": "PostToolUse",
	"pre-shell": "PreToolUse",
	"stop":      "Stop",
}

// hookLintBudget bounds the lint a post-edit event runs; past it the hook
// says nothing.
var hookLintBudget = 60 * time.Second

// hookNoteLines caps the lint output a post-edit note carries.
const hookNoteLines = 40

// hookInput is the part of an agent's hook event gotest reads.
type hookInput struct {
	HookEventName  string `json:"hook_event_name"`
	Cwd            string `json:"cwd"`
	StopHookActive bool   `json:"stop_hook_active"`
	ToolInput      struct {
		FilePath string `json:"file_path"`
		Command  string `json:"command"`
	} `json:"tool_input"`
}

type hookContext struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type hookOutput struct {
	HookSpecificOutput hookContext `json:"hookSpecificOutput"`
}

// runHook answers one agent hook event. It never exits 2, which agents
// read as a block: a usage or input error exits 1. Only the stop event
// answers with a decision, one that continues the agent's turn.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: gotest hook post-edit|pre-shell|stop < event.json")
		return 1
	}
	event, known := hookEvents[args[0]]
	if !known {
		fmt.Fprintf(stderr, "FAIL: unknown hook event %q; gotest hook takes post-edit, pre-shell or stop\n", args[0])
		return 1
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: reading the hook event: %v\n", err)
		return 1
	}
	var in hookInput
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &in); err != nil {
			fmt.Fprintf(stderr, "FAIL: the hook event on stdin is not JSON: %v\n", err)
			return 1
		}
	}
	if in.Cwd == "" {
		in.Cwd, _ = os.Getwd()
	}
	var note string
	switch args[0] {
	case "stop":
		reason := stopReason(in)
		if reason == "" {
			return 0
		}
		return writeHookJSON(stdout, stderr, hookDecision{Decision: "block", Reason: reason})
	case "post-edit":
		note = postEditNote(in)
	case "pre-shell":
		note = preShellNote(in)
	}
	if note == "" {
		return 0
	}
	if in.HookEventName != "" {
		event = in.HookEventName
	}
	return writeHookJSON(stdout, stderr, hookOutput{HookSpecificOutput: hookContext{HookEventName: event, AdditionalContext: note}})
}

// writeHookJSON writes a hook's answer as one JSON object.
func writeHookJSON(stdout, stderr io.Writer, answer any) int {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(answer); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1
	}
	return 0
}

// postEditNote lints the package of an edited test file in a module that
// uses gotest and returns what lint reports; "" when it is clean, when it
// cannot finish in time or when the file is none of gotest's business.
func postEditNote(in hookInput) string {
	file := in.ToolInput.FilePath
	if !strings.HasSuffix(file, "_test.go") || isGeneratedHarness(file) {
		return ""
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(in.Cwd, file)
	}
	if _, err := os.Stat(file); err != nil {
		return ""
	}
	mod := findModule(filepath.Dir(file))
	if mod == nil || !mod.usesGotest() {
		return ""
	}
	rel, err := filepath.Rel(mod.root, filepath.Dir(file))
	if err != nil {
		return ""
	}
	pattern := "./" + filepath.ToSlash(rel)
	if rel == "." {
		pattern = "."
	}
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), hookLintBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "lint", pattern) //nolint:gosec // G204: this executable, on a package of the agent's module
	cmd.Dir = mod.root
	// A hook never downloads: a module not in the cache fails the load.
	cmd.Env = append(os.Environ(), "GOPROXY=off")
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	var exitErr *exec.ExitError
	if err == nil || ctx.Err() != nil || !errors.As(err, &exitErr) {
		return ""
	}
	text := strings.TrimSpace(out.String())
	if text == "" || strings.Contains(text, "GOPROXY=off") {
		return ""
	}
	return fmt.Sprintf("gotest lint %s, the package of the test file just edited, reports:\n%s\n`gotest help lint` explains each rule.", pattern, capLines(text, hookNoteLines))
}

// capLines keeps the first n lines of text and counts the rest.
func capLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… and %d more lines", len(lines)-n)
}

// isGeneratedHarness reports whether file is one gotest generate writes.
func isGeneratedHarness(file string) bool {
	base := filepath.Base(file)
	return base == "gotest_psuite_test.go" || base == "gotest_pxsuite_test.go"
}

// hookModule is the module a directory belongs to.
type hookModule struct {
	root string
	file *modfile.File
}

// findModule returns the module of dir, nil outside one.
func findModule(dir string) *hookModule {
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod")) //nolint:gosec // G304: the go.mod above a path the agent named
		if err == nil {
			// ParseLax drops the tool directives; it reads a go.mod newer
			// than this parser.
			f, err := modfile.Parse("go.mod", data, nil)
			if err != nil {
				f, err = modfile.ParseLax("go.mod", data, nil)
			}
			if err != nil || f.Module == nil {
				return nil
			}
			return &hookModule{root: dir, file: f}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func (m *hookModule) path() string { return m.file.Module.Mod.Path }

// usesGotest reports whether the module is gotest or requires it.
func (m *hookModule) usesGotest() bool {
	if m.path() == gotestModule {
		return true
	}
	for _, r := range m.file.Require {
		if r.Mod.Path == gotestModule {
			return true
		}
	}
	return false
}

// command is how the module runs gotest from dir: go tool where go.mod
// declares it, go run in gotest's own module, else the gotest on PATH.
func (m *hookModule) command(dir string) string {
	for _, t := range m.file.Tool {
		if t.Path == gotestModule+"/cmd/gotest" {
			return "go tool gotest"
		}
	}
	if m.path() == gotestModule {
		if filepath.Clean(dir) == filepath.Clean(m.root) {
			return "go run ./cmd/gotest"
		}
		return "go run " + gotestModule + "/cmd/gotest"
	}
	return "gotest"
}

// preShellNote returns a note for a shell command that runs go test over
// packages declaring gotest suites; "" for any other command.
func preShellNote(in hookInput) string {
	line := in.ToolInput.Command
	if !strings.Contains(line, "test") {
		return ""
	}
	dir := in.Cwd
	for _, seg := range shellSegments(line) {
		words := stripCommandPrefix(seg)
		if len(words) == 0 {
			continue
		}
		switch {
		case words[0].text == "cd":
			dir = cdTarget(dir, words[1:])
		case len(words) >= 2 && words[0].text == "go" && words[1].text == "test":
			if dir == "" {
				return ""
			}
			mod := findModule(dir)
			if mod == nil || !mod.usesGotest() || !runsSuites(mod, dir, words[2:]) {
				return ""
			}
			rewritten := line[:words[0].start] + mod.command(dir) + line[words[1].end:]
			return fmt.Sprintf("This module's tests are gotest suites, and go test does not run them: without the harness gotest generates, a suite package runs none of its suites and can still report ok. Run them with:\n%s", rewritten)
		}
	}
	return ""
}

// cdTarget is the directory cd args leads to from dir; "" when it cannot
// be told.
func cdTarget(dir string, args []shellWord) string {
	if dir == "" || len(args) != 1 {
		return ""
	}
	to := args[0].text
	if to == "" || to == "-" || strings.HasPrefix(to, "~") || strings.Contains(to, "$") {
		return ""
	}
	if !filepath.IsAbs(to) {
		to = filepath.Join(dir, to)
	}
	return to
}

// goTestValueFlags are the go test flags that take their value as the next
// word.
var goTestValueFlags = map[string]bool{
	"run": true, "skip": true, "count": true, "timeout": true, "tags": true, "p": true,
	"parallel": true, "cpu": true, "bench": true, "benchtime": true, "coverprofile": true,
	"covermode": true, "coverpkg": true, "cpuprofile": true, "memprofile": true,
	"blockprofile": true, "mutexprofile": true, "trace": true, "outputdir": true,
	"exec": true, "ldflags": true, "gcflags": true, "asmflags": true, "mod": true,
	"modfile": true, "o": true, "fuzz": true, "fuzztime": true, "fuzzminimizetime": true,
	"shuffle": true, "vet": true, "list": true, "C": true, "pgo": true, "overlay": true,
	"toolexec": true, "installsuffix": true, "pkgdir": true, "buildmode": true,
	"compiler": true, "gccgoflags": true, "memprofilerate": true, "blockprofilerate": true,
	"mutexprofilefraction": true,
}

// goTestBuildOnly are the go test flags under which no test runs, or which
// gotest leaves to other commands.
var goTestBuildOnly = map[string]bool{"c": true, "n": true, "o": true, "fuzz": true}

// runsSuites reports whether go test args, run in dir, reach a package of
// mod that declares a gotest suite.
func runsSuites(mod *hookModule, dir string, args []shellWord) bool {
	var patterns []string
	for i := 0; i < len(args); i++ {
		a := args[i].text
		if a == "-args" || a == "--args" {
			break
		}
		if !strings.HasPrefix(a, "-") {
			patterns = append(patterns, a)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if goTestBuildOnly[name] {
			return false
		}
		if goTestValueFlags[name] && !hasValue && i+1 < len(args) {
			i++
			value = args[i].text
		}
		if name == "C" && filepath.IsAbs(value) {
			dir = value
		} else if name == "C" {
			dir = filepath.Join(dir, value)
		}
	}
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	for _, p := range patterns {
		target, recursive, ok := patternDir(mod, dir, p)
		if ok && declaresSuites(mod.root, target, recursive) {
			return true
		}
	}
	return false
}

// patternDir is the directory a package pattern names inside mod, and
// whether it reaches the directories below; ok is false for a pattern
// outside mod, such as a standard-library package.
func patternDir(mod *hookModule, dir, pattern string) (target string, recursive, ok bool) {
	if strings.HasSuffix(pattern, ".go") {
		return filepath.Dir(filepath.Join(dir, pattern)), false, true
	}
	p, recursive := strings.CutSuffix(pattern, "/...")
	if p == "..." {
		p, recursive = ".", true
	}
	switch {
	case p == "." || p == ".." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../"):
		target = filepath.Join(dir, p)
	case filepath.IsAbs(p):
		target = p
	case p == mod.path():
		target = mod.root
	case strings.HasPrefix(p, mod.path()+"/"):
		target = filepath.Join(mod.root, strings.TrimPrefix(p, mod.path()+"/"))
	default:
		return "", false, false
	}
	return target, recursive, true
}

// suiteImport and suiteType are what a test file declaring a suite holds.
var (
	suiteImport = []byte(`"` + gotestModule + `/pkg/gotest"`)
	suiteType   = regexp.MustCompile(`\b[A-Za-z_]\w*TestSuite\s+struct\b`)
)

// declaresSuites reports whether a test file of the package in dir, or of
// one below it when recursive, imports gotest and declares a suite. It
// stays inside the module rooted at root, as go test patterns do.
func declaresSuites(root, dir string, recursive bool) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if path == dir {
				return nil
			}
			name := d.Name()
			if !recursive || name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") || isGeneratedHarness(path) {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: a test file of the agent's module
		if err == nil && bytes.Contains(data, suiteImport) && suiteType.Match(data) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// shellWord is one word of a shell command and where it sits in the line.
type shellWord struct {
	text       string
	start, end int
}

// shellSegments splits a command line into its simple commands at &&, ||,
// ;, |, & and newlines, each a list of words with quotes removed. It reads
// the line the way sh does closely enough to find a go test invocation.
func shellSegments(line string) [][]shellWord {
	var (
		segs  [][]shellWord
		words []shellWord
		cur   strings.Builder
		start = -1
	)
	endWord := func(at int) {
		if start >= 0 {
			words = append(words, shellWord{text: cur.String(), start: start, end: at})
		}
		cur.Reset()
		start = -1
	}
	endSeg := func(at int) {
		endWord(at)
		if len(words) > 0 {
			segs = append(segs, words)
		}
		words = nil
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\'' || c == '"':
			if start < 0 {
				start = i
			}
			j := i + 1
			for j < len(line) && line[j] != c {
				if c == '"' && line[j] == '\\' && j+1 < len(line) {
					j++
				}
				cur.WriteByte(line[j])
				j++
			}
			i = j
		case c == '\\' && i+1 < len(line):
			if start < 0 {
				start = i
			}
			if line[i+1] != '\n' {
				cur.WriteByte(line[i+1])
			}
			i++
		case c == ' ' || c == '\t':
			endWord(i)
		case c == ';' || c == '&' || c == '|' || c == '\n' || c == '(' || c == ')':
			endSeg(i)
		default:
			if start < 0 {
				start = i
			}
			cur.WriteByte(c)
		}
	}
	endSeg(len(line))
	return segs
}

// envAssignment matches a NAME=value word before a command.
var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// stripCommandPrefix drops the environment assignments, an env command and
// the keywords that can stand before a simple command.
func stripCommandPrefix(words []shellWord) []shellWord {
	for len(words) > 0 {
		w := words[0].text
		switch {
		case envAssignment.MatchString(w), w == "env", w == "exec", w == "time", w == "then", w == "do", w == "else", w == "!", w == "{":
			words = words[1:]
		default:
			return words
		}
	}
	return words
}

func printHookHelp() {
	fmt.Print(`gotest hook — answer a coding agent's hook event

Usage:
  gotest hook <event> < event.json

Reads the agent's hook event as JSON on stdin. When there is something to
say, writes one JSON object: for post-edit and pre-shell, hookSpecificOutput
carries additionalContext for the agent, and no permission decision; for
stop, decision "block" with a reason, which continues the agent's turn.
Otherwise writes nothing. Exits 0, or 1 on an unknown event or input that
is not JSON; never 2, which agents read as a block.

Events:
  post-edit   After a file edit: for a _test.go file in a module that uses
              gotest, the lint findings of its package
  pre-shell   Before a shell command: for go test over packages that declare
              gotest suites, a note naming the gotest command that runs them
  stop        When the agent ends its turn, in a git checkout of a module
              that uses gotest: the weakenings gotest audit finds in the
              working tree against the merge base of HEAD and origin's
              default branch, else HEAD, each reported once per checkout.
              Nothing when stop_hook_active is set, or when the audit fails
              or takes longer than 20s

The gotest plugin for Claude Code wires these events.
`)
}
