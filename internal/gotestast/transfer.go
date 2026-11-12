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
	// a non-struct embedded type is a value of its own, never transferred
	if embedded := derefOnce(f.Type()); !isStruct(embedded) {
		typ := types.TypeString(f.Type(), types.RelativeTo(in))
		if types.IsInterface(embedded) {
			return At(f.Pos(), fmt.Errorf("shared fixture %q embeds %s, an interface the state cannot carry, so the suites read nil; assign it in Hydrate", fixture, typ))
		}
		return At(f.Pos(), fmt.Errorf("shared fixture %q embeds %s, whose value is not transferred to the suites; hold it in a named field (%s %s), or assign it in Hydrate", fixture, typ, f.Name(), typ))
	}
	fields := promotedExported(f.Type(), hydrated, map[types.Type]bool{})
	var missing []string
	for _, name := range fields {
		if !hydrated[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	typ := types.TypeString(f.Type(), types.RelativeTo(in))
	if len(missing) < len(fields) {
		return At(f.Pos(), fmt.Errorf("shared fixture %q embeds %s, whose exported fields %s are neither transferred to the suites nor assigned in Hydrate; hold it in a named field (%s %s), or assign them in Hydrate", fixture, typ, joinAnd(missing), f.Name(), typ))
	}
	return At(f.Pos(), fmt.Errorf("shared fixture %q embeds %s, whose exported fields are not transferred to the suites; hold it in a named field (%s %s), or assign it in Hydrate", fixture, typ, f.Name(), typ))
}

// derefOnce is t through an alias and one pointer.
func derefOnce(t types.Type) types.Type {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		return types.Unalias(ptr.Elem())
	}
	return t
}

func isStruct(t types.Type) bool {
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

// joinAnd lists names as "a", "a and b", "a, b and c".
func joinAnd(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// promotedExported names the exported fields embedding t promotes: those of
// a struct, through one pointer, and of the structs it embeds, except the
// ones under an embedded field hydrated names. A non-struct promotes none.
func promotedExported(t types.Type, hydrated map[string]bool, seen map[types.Type]bool) []string {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	if seen[t] {
		return nil
	}
	seen[t] = true
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	var names []string
	for i := range st.NumFields() {
		f := st.Field(i)
		switch {
		case f.Anonymous():
			if !hydrated[f.Name()] {
				names = append(names, promotedExported(f.Type(), hydrated, seen)...)
			}
		case f.Exported():
			names = append(names, f.Name())
		}
	}
	return names
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
