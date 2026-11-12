package gotestast

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
)

// EmbeddedFieldError refuses an embedded field of the shared fixture named
// fixture whose exported fields would not reach the suites. hydrated names
// the fields Hydrate assigns; in qualifies the type's name.
func EmbeddedFieldError(fixture string, f *types.Var, hydrated map[string]bool, in *types.Package) error {
	if !f.Anonymous() || hydrated[f.Name()] {
		return nil
	}
	// a fixture embedded in a fixture is refused by the fixture rules
	if n := namedBehind(f.Type()); n != nil && strings.HasSuffix(n.Obj().Name(), protocol.SuffixFixture) {
		return nil
	}
	if !promotesExported(f.Type(), map[types.Type]bool{}) {
		return nil
	}
	typ := types.TypeString(f.Type(), types.RelativeTo(in))
	return At(f.Pos(), fmt.Errorf("shared fixture %q embeds %s, whose exported fields are not transferred to the suites; hold it in a named field (%s %s), or assign it in Hydrate", fixture, typ, f.Name(), typ))
}

// promotesExported reports whether embedding t promotes an exported field:
// a struct, through one pointer, with an exported named field or an embedded
// field that promotes one. A non-struct type promotes none.
func promotesExported(t types.Type, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := range st.NumFields() {
		f := st.Field(i)
		if f.Anonymous() {
			if promotesExported(f.Type(), seen) {
				return true
			}
			continue
		}
		if f.Exported() {
			return true
		}
	}
	return false
}
