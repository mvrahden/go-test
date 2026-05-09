package assert_test

import (
	"reflect"
	"sync"
	"testing"
)

func TestGoTestingInternalsCompatible(t *testing.T) {
	v := reflect.ValueOf(t).Elem()
	check := func(name string, wantType reflect.Type) {
		t.Helper()
		f := v.FieldByName(name)
		if !f.IsValid() {
			t.Fatalf("testing.T missing field %q — Go internals changed", name)
		}
		if !f.CanAddr() {
			t.Fatalf("testing.T field %q is not addressable", name)
		}
		if f.Type() != wantType {
			t.Fatalf("testing.T field %q type changed: want %v, got %v", name, wantType, f.Type())
		}
	}
	check("mu", reflect.TypeFor[sync.RWMutex]())
	check("helperPCs", reflect.TypeFor[map[uintptr]struct{}]())
	check("helperNames", reflect.TypeFor[map[string]struct{}]())
}
