package gotestast

import "go/types"

const testifySuitePkg = "github.com/stretchr/testify/suite"

// IsTestifySuite reports whether t embeds testify's suite.Suite, at any
// depth: testify runs it, not gotest.
func IsTestifySuite(t types.Type) bool {
	return embedsTestifySuite(t, map[types.Type]bool{})
}

func embedsTestifySuite(t types.Type, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj.Name() == "Suite" && obj.Pkg() != nil && obj.Pkg().Path() == testifySuitePkg {
			return true
		}
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
		if f := st.Field(i); f.Embedded() && embedsTestifySuite(f.Type(), seen) {
			return true
		}
	}
	return false
}
