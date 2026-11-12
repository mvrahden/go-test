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

// TransferFieldError refuses a transfer field of the shared fixture named
// fixture whose type JSON cannot carry to the suites and back.
func TransferFieldError(fixture string, f *types.Var) error {
	if reason := nonSerializable(f.Type(), map[*types.Named]bool{}); reason != "" {
		return At(f.Pos(), fmt.Errorf("shared fixture %q: transfer field %q has non-JSON-serializable type %s (%s) — assign it in Hydrate to make it a local field, or unexport it when only the setup process uses it", fixture, f.Name(), f.Type(), reason))
	}
	return nil
}

// nonSerializable names what in t JSON cannot carry, or "". A named type is
// read once: a type that refers to itself is read no deeper.
func nonSerializable(t types.Type, seen map[*types.Named]bool) string {
	if n, ok := types.Unalias(t).(*types.Named); ok {
		if seen[n] {
			return ""
		}
		seen[n] = true
	}
	switch u := t.Underlying().(type) {
	case *types.Chan:
		return "channel"
	case *types.Signature:
		return "function"
	case *types.Interface:
		if !u.Empty() {
			return "interface with methods"
		}
	case *types.Array:
		return nonSerializable(u.Elem(), seen)
	case *types.Slice:
		return nonSerializable(u.Elem(), seen)
	case *types.Map:
		if r := nonSerializable(u.Key(), seen); r != "" {
			return r + " in map key"
		}
		if !isJSONMapKey(u.Key()) {
			return "non-string/integer map key"
		}
		return nonSerializable(u.Elem(), seen)
	case *types.Pointer:
		return nonSerializable(u.Elem(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			if !f.Exported() {
				continue
			}
			if r := nonSerializable(f.Type(), seen); r != "" {
				return r + " in field " + f.Name()
			}
		}
	}
	return ""
}

func isJSONMapKey(t types.Type) bool {
	if u, ok := t.Underlying().(*types.Basic); ok {
		return u.Info()&(types.IsString|types.IsInteger) != 0
	}
	return false
}
