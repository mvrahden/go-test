package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/skills"
)

// defaultSkillsDir is the project skills directory most agents share;
// Claude Code reads .claude/skills instead.
const defaultSkillsDir = ".agents/skills"

func runSkill(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	args := inv.Args
	if len(args) == 0 || args[0] != "install" {
		if len(args) > 0 {
			fmt.Fprintf(os.Stderr, "unknown skill command: %s\n", args[0])
		}
		fmt.Fprintln(os.Stderr, `usage: gotest skill install [--dir=<skills-dir>]`)
		return 2
	}

	ownArgs, rest, err := SplitArgs(args[1:], skillAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 2
	}
	if len(rest) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: unexpected argument %s — skill install accepts only --dir\n", rest[0])
		return 2
	}
	dir := extractStringFlag(ownArgs, "--dir", "")
	if dir == "" {
		if hasFlag(ownArgs, "--dir") {
			fmt.Fprintln(os.Stderr, "FAIL: --dir needs a directory")
			return 2
		}
		dir = defaultSkillsDir
	}

	target := filepath.Join(dir, skills.Name)
	if err := installSkill(target); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 2
	}
	fmt.Println(target)
	return 0
}

// installSkill replaces target with the embedded skill. It refuses a
// directory that holds some other skill, never merging into it. The new copy
// is written beside target and swapped in once complete, so a failed install
// leaves the old copy as it was. Both copies wait outside the skills
// directory, so an interrupted run never leaves agents a second skill.
func installSkill(target string) error {
	if err := checkReplaceable(target); err != nil {
		return err
	}
	skillsDir := filepath.Dir(target)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil { //nolint:gosec // G301: a skills directory is read by other tools
		return err
	}
	staged, err := os.MkdirTemp(filepath.Dir(skillsDir), "."+skills.Name+"-new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged) //nolint:errcheck // best effort: gone after the swap
	if err := writeSkill(staged); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil { //nolint:gosec // G302: read by other tools; MkdirTemp creates 0700
		return err
	}
	old := ""
	if _, err := os.Lstat(target); err == nil {
		old = staged + "-old"
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}
	if err := os.Rename(staged, target); err != nil {
		if old != "" {
			_ = os.Rename(old, target)
		}
		return err
	}
	if old != "" {
		if err := os.RemoveAll(old); err != nil {
			fmt.Fprintf(os.Stderr, "warning: the replaced copy is left at %s: %v\n", old, err)
		}
	}
	return nil
}

// writeSkill writes the embedded skill's files into dir.
func writeSkill(dir string) error {
	src, err := fs.Sub(skills.FS, skills.Name)
	if err != nil {
		return err
	}
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755) //nolint:gosec // G301: a skills directory is read by other tools
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644) //nolint:gosec // G306: skill files are read by other tools
	})
}

// checkReplaceable accepts a missing target or one whose SKILL.md names
// the embedded skill.
func checkReplaceable(target string) error {
	data, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Lstat(target); statErr == nil {
			return fmt.Errorf("%s exists and holds no SKILL.md; remove it first", target)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if name := frontmatterName(data); name != skills.Name {
		return fmt.Errorf("%s holds skill %q, not %s; remove it first", target, name, skills.Name)
	}
	return nil
}

// frontmatterName returns the name field of a SKILL.md frontmatter.
func frontmatterName(data []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return ""
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
