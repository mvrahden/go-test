package lint

import (
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/refactor"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
)

// unfocusEdits removes the F_ prefix at the focused name declared at decl
// and at every reference to it in the pass.
func unfocusEdits(pass *analysis.Pass, decl token.Pos) []analysis.TextEdit {
	edits := []analysis.TextEdit{unfocus(decl)}
	target, ok := refactor.DeclaredAt(pass.TypesInfo, decl)
	if !ok {
		return edits
	}
	for _, id := range refactor.Idents(pass.Files, pass.TypesInfo, target) {
		if id.Pos() != decl {
			edits = append(edits, unfocus(id.Pos()))
		}
	}
	return edits
}

func unfocus(pos token.Pos) analysis.TextEdit {
	return analysis.TextEdit{Pos: pos, End: pos + token.Pos(len(protocol.PrefixFocused)), NewText: []byte("")}
}

// fileEdit is a fix's edit with the file it lands in and that file's
// package, whose imports name the file's imports.
type fileEdit struct {
	tf   *token.File
	pkg  *types.Package
	edit analysis.TextEdit
}

// focusReferences returns the edits a focus fix found in act needs in the
// other variants of its package among roots: the external test package
// refers to a suite of the internal one, which act's pass cannot see. own
// are act's edits, the declaration's among them; an edit they hold is not
// repeated.
func focusReferences(roots []*checker.Action, act *checker.Action, own []analysis.TextEdit) []fileEdit {
	fset := act.Package.Fset
	var target refactor.Object
	var dir string
	for _, e := range own {
		if t, ok := refactor.DeclaredAt(act.Package.TypesInfo, e.Pos); ok {
			target, dir = t, filepath.Dir(fset.File(e.Pos).Name())
			break
		}
	}
	if dir == "" {
		return nil
	}
	have := map[token.Position]bool{}
	for _, e := range own {
		have[fset.Position(e.Pos)] = true
	}
	var out []fileEdit
	for _, r := range roots {
		p := r.Package
		if r == act || isTestMain(p) || p.TypesInfo == nil || !slices.ContainsFunc(p.CompiledGoFiles, func(f string) bool { return filepath.Dir(f) == dir }) {
			continue
		}
		for _, id := range refactor.Idents(p.Syntax, p.TypesInfo, target) {
			at := p.Fset.Position(id.Pos())
			if have[at] {
				continue
			}
			have[at] = true
			out = append(out, fileEdit{tf: p.Fset.File(id.Pos()), pkg: p.Types, edit: unfocus(id.Pos())})
		}
	}
	return out
}

// constrainedFocusReferences returns the edits a focus fix found in act
// needs in the files of its directory that the load under tags left out for
// their build constraint, each found in a load under that constraint's
// tags; overlay holds the sources earlier fixes changed. It errs when such
// a file spells the focused name and no load can resolve it.
func constrainedFocusReferences(roots []*checker.Action, act *checker.Action, own []analysis.TextEdit, tags string, overlay map[string][]byte) ([]fileEdit, error) {
	fset := act.Package.Fset
	var target refactor.Object
	var dir string
	for _, e := range own {
		if t, ok := refactor.DeclaredAt(act.Package.TypesInfo, e.Pos); ok {
			target, dir = t, filepath.Dir(fset.File(e.Pos).Name())
			break
		}
	}
	if dir == "" {
		return nil, nil
	}
	var loaded, ignored []string
	for _, r := range roots {
		loaded = append(loaded, r.Package.CompiledGoFiles...)
		loaded = append(loaded, r.Package.GoFiles...)
	}
	for _, r := range roots {
		for _, f := range r.Package.IgnoredFiles {
			if filepath.Dir(f) == dir && strings.HasSuffix(f, ".go") && !slices.Contains(loaded, f) && !slices.Contains(ignored, f) {
				ignored = append(ignored, f)
			}
		}
	}
	if len(ignored) == 0 {
		return nil, nil
	}
	refs, err := refactor.ConstrainedRefs(dir, ignored, tags, overlay, target)
	if err != nil {
		return nil, err
	}
	out := make([]fileEdit, 0, len(refs))
	for _, r := range refs {
		out = append(out, fileEdit{tf: r.File, pkg: r.Pkg, edit: unfocus(r.Pos)})
	}
	return out, nil
}

// buildTags is the comma-separated -tags value of a load's build flags.
func buildTags(flags []string) string {
	var tags []string
	for _, f := range flags {
		if v, ok := strings.CutPrefix(strings.TrimLeft(f, "-"), "tags="); ok {
			tags = append(tags, v)
		}
	}
	return strings.Join(tags, ",")
}
