package refactor

import (
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"slices"
	"sort"
	"strings"
)

// Ref is an identifier a load under other build tags found.
type Ref struct {
	File *token.File
	Pos  token.Pos
	Pkg  *types.Package
}

// ConstrainedRefs returns the identifiers referring to target in files, Go
// files of dir a load under tags left out for their build constraint. Each
// is type-checked in a load of dir under tags and the tags its //go:build
// line names, overlay replacing the contents of the files it names. A file
// no such load type-checks that spells target's name is refused, at the
// position: what the name refers to there is unknown.
func ConstrainedRefs(dir string, files []string, tags string, overlay map[string][]byte, target Object) ([]Ref, error) {
	byTags := map[string][]string{}
	for _, file := range files {
		own, err := constraintTags(file)
		if err != nil {
			return nil, err
		}
		byTags[own] = append(byTags[own], file)
	}
	var refs []Ref
	var unchecked []parsedFile
	type at struct {
		file   string
		offset int
	}
	seen := map[at]bool{}
	for own, group := range byTags {
		if own != "" {
			for _, p := range loadTaggedOver(dir, joinTags(tags, own), overlay) {
				if p.TypesInfo == nil || len(p.Errors) > 0 {
					continue
				}
				v := checked{p.Fset, p.Syntax, p.TypesInfo, p.Types}
				var known []string
				for _, f := range group {
					if name := fileNamed(v, f); name != "" {
						known = append(known, name)
					}
				}
				for _, id := range Idents(v.files, v.info, target) {
					tf := v.fset.File(id.Pos())
					key := at{tf.Name(), tf.Offset(id.Pos())}
					if !slices.Contains(known, tf.Name()) || seen[key] {
						continue
					}
					seen[key] = true
					refs = append(refs, Ref{File: tf, Pos: id.Pos(), Pkg: v.pkg})
				}
				group = slices.DeleteFunc(group, func(f string) bool { return fileNamed(v, f) != "" })
			}
		}
		for _, f := range group {
			src, ok := overlay[f]
			if !ok {
				var err error
				if src, err = os.ReadFile(f); err != nil {
					return nil, err
				}
			}
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, f, src, parser.SkipObjectResolution)
			if err != nil {
				continue // a file that does not parse is no file go builds
			}
			unchecked = append(unchecked, parsedFile{fset, parsed})
		}
	}
	sort.Slice(unchecked, func(i, j int) bool {
		return unchecked[i].fset.File(unchecked[i].file.FileStart).Name() < unchecked[j].fset.File(unchecked[j].file.FileStart).Name()
	})
	if err := unresolved(unchecked, target); err != nil {
		return nil, err
	}
	return refs, nil
}

// joinTags is the sorted union of two comma-separated tag lists.
func joinTags(a, b string) string {
	var tags []string
	for _, t := range strings.Split(a+","+b, ",") {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	sort.Strings(tags)
	return strings.Join(tags, ",")
}
