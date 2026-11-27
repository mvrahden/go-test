package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// hookStopBudget bounds the audit a stop event runs; past it the hook says
// nothing. An audit of gotest's own checkout takes 5 to 8 seconds.
var hookStopBudget = 20 * time.Second

// hookStopShown caps the weakenings a stop reason lists.
const hookStopShown = 20

// hookDecision is a stop hook's answer that keeps the agent going: Claude
// Code and Codex both continue the turn with reason as its prompt.
type hookDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// hookState is what the stop hook told an agent about a checkout: the ids
// of the weakenings it reported that still stand.
type hookState struct {
	Reported []string `json:"reported"`
}

// stopReason audits the working tree of a gotest module's checkout against
// its base and returns the weakenings not reported before, as the reason
// to continue; "" when there are none, when a stop hook already continued
// the turn, and on any failure.
func stopReason(in hookInput) string {
	if in.StopHookActive || in.Cwd == "" {
		return ""
	}
	mod := findModule(in.Cwd)
	if mod == nil || !mod.usesGotest() {
		return ""
	}
	top, err := gitOutput(mod.root, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	base, label := stopBase(mod.root)
	if base == "" {
		return ""
	}
	doc := stopAudit(mod.root, base)
	if doc == nil {
		return ""
	}
	stateFile, err := gotestrunner.HookStateFile(top)
	if err != nil {
		return ""
	}
	var state hookState
	if data, err := os.ReadFile(stateFile); err == nil { //nolint:gosec // G304: the hook's own state file
		_ = json.Unmarshal(data, &state)
	}
	var fresh []*gotestaudit.Finding
	var standing []string
	for i := range doc.Findings {
		f := &doc.Findings[i]
		if !f.Weakens || slices.Contains(standing, f.ID) {
			continue
		}
		if !slices.Contains(state.Reported, f.ID) {
			fresh = append(fresh, f)
		}
		standing = append(standing, f.ID)
	}
	kept := hookState{Reported: []string{}}
	for _, id := range standing {
		if len(fresh) > 0 || slices.Contains(state.Reported, id) {
			kept.Reported = append(kept.Reported, id)
		}
	}
	// What cannot be remembered is not said: it would come back every turn.
	if err := writeHookState(stateFile, kept); err != nil || len(fresh) == 0 {
		return ""
	}
	return stopText(doc, fresh, base, label)
}

// stopBase is the commit the stop hook audits against: the merge base of
// HEAD and the default branch origin names, else HEAD; "" without a commit.
func stopBase(dir string) (commit, label string) {
	head, err := gitOutput(dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return "", ""
	}
	if ref, err := gitOutput(dir, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		if mb, err := gitOutput(dir, "merge-base", "HEAD", ref); err == nil && mb != "" {
			return mb, "the merge base with " + strings.TrimPrefix(ref, "refs/remotes/")
		}
	}
	return head, "HEAD"
}

// stopAudit runs this CLI's audit of the module's working tree against
// base and reads its document; nil when it fails or overruns the budget.
func stopAudit(root, base string) *gotestaudit.Document {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), hookStopBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "audit", "--base="+base, "--format=json", "--agent=false") //nolint:gosec // G204: this executable, on the agent's module
	cmd.Dir = root
	// A hook never downloads: a module not in the cache fails the load.
	cmd.Env = append(os.Environ(), "GOPROXY=off")
	// An interrupt lets the audit stop the base's recording it started.
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	var exitErr *exec.ExitError
	// Exit 2 with a document is a package that does not compile, the
	// others compared.
	if ctx.Err() != nil || (err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 2)) {
		return nil
	}
	var doc gotestaudit.Document
	if json.Unmarshal(out.Bytes(), &doc) != nil || doc.Schema == 0 {
		return nil
	}
	return &doc
}

// writeHookState replaces the state file whole.
func writeHookState(path string, state hookState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
	}
	return werr
}

// stopText is the reason a stop hook gives for the weakenings found.
func stopText(doc *gotestaudit.Document, fresh []*gotestaudit.Finding, base, label string) string {
	var b strings.Builder
	short := base
	if len(short) > 12 {
		short = short[:12]
	}
	fmt.Fprintf(&b, "gotest audit: this change weakens the test estate against %s (%s), in %d finding(s) not reported before:\n", short, label, len(fresh))
	for i, f := range fresh {
		if i == hookStopShown {
			fmt.Fprintf(&b, "- … and %d more\n", len(fresh)-i)
			break
		}
		fmt.Fprintf(&b, "- %s: %s\n", f.Class, gotestaudit.FindingText(doc, f))
	}
	fmt.Fprintf(&b, "For each one, either undo it, or state it plainly to the user in your final message: what got weaker and why. Never keep a weakening silently. `gotest audit --base=%s` lists them all.", short)
	return b.String()
}
