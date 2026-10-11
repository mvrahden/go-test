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
		return 1
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
// directory that holds some other skill, never merging into it.
func installSkill(target string) error {
	if err := checkReplaceable(target); err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	src, err := fs.Sub(skills.FS, skills.Name)
	if err != nil {
		return err
	}
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(target, filepath.FromSlash(path))
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
