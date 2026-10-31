package gotestgen

import (
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
)

// Generated files import every package under an ƒ alias, so no name the
// package itself declares, and no two packages of the same name, collide
// with an import.
var fixedAliases = map[string]string{
	"context":                         "ƒcontext",
	"math":                            "ƒmath",
	"strconv":                         "ƒstrconv",
	"strings":                         "ƒstrings",
	"testing":                         "ƒtesting",
	"time":                            "ƒtime",
	about.Repo + "/pkg/gotest":        "ƒgotest",
	about.Repo + "/pkg/gotestfuzz":    "ƒgotestfuzz",
	about.Repo + "/pkg/gotestruntime": "ƒgotestruntime",
}

// importSet hands out the aliases of one generated file and records the
// imports it uses.
type importSet struct {
	self    string            // the generated file's own package path
	aliases map[string]string // path → alias
	taken   map[string]bool
}

func newImportSet(self string) *importSet {
	return &importSet{self: self, aliases: map[string]string{}, taken: map[string]bool{}}
}

// use imports path and returns its alias; name is the package's own name.
// Packages outside the fixed set are numbered per name: ƒpg0, ƒpg1.
func (s *importSet) use(path, name string) string {
	if a, ok := s.aliases[path]; ok {
		return a
	}
	a, ok := fixedAliases[path]
	if !ok {
		for i := 0; ; i++ {
			a = "ƒ" + name + strconv.Itoa(i)
			if !s.taken[a] {
				break
			}
		}
	}
	s.aliases[path] = a
	s.taken[a] = true
	return a
}

// std imports one of the fixed packages and returns its alias.
func (s *importSet) std(path string) string { return s.use(path, "") }

// qualifier qualifies type names by alias, importing what it names.
func (s *importSet) qualifier() types.Qualifier {
	return func(p *types.Package) string {
		if p == nil || p.Path() == s.self {
			return ""
		}
		return s.use(p.Path(), p.Name())
	}
}

// typeRef renders t as the generated file refers to it.
func (s *importSet) typeRef(t types.Type) string { return types.TypeString(t, s.qualifier()) }

// imports lists the used imports, the standard library's first, each
// group sorted by path.
func (s *importSet) imports() []headerImport {
	out := make([]headerImport, 0, len(s.aliases))
	for path, alias := range s.aliases {
		out = append(out, headerImport{Name: alias, Path: path})
	}
	sort.Slice(out, func(i, j int) bool {
		if si, sj := isStdPath(out[i].Path), isStdPath(out[j].Path); si != sj {
			return si
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// isStdPath reports whether path looks like a standard library import.
func isStdPath(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}
