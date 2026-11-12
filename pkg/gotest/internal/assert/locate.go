package assert

import (
	"bytes"
	"reflect"
)

// firstDifference is where x and y first differ in the order they render
// in, as a path such as [3].Name; "" at the top. Its notion of a
// difference is reflect.DeepEqual's.
func firstDifference(x, y any) *step {
	at, _, _ := firstDifferenceValues(x, y)
	return at
}

// firstDifferenceValues is [firstDifference] with the two values found
// there; a side that has no element or entry there is the zero Value.
func firstDifferenceValues(x, y any) (at *step, xv, yv reflect.Value) {
	l := &locator{seen: map[seenPair]bool{}, r: &renderer{onPath: map[visit]bool{}}}
	at, _ = l.find(reflect.ValueOf(x), reflect.ValueOf(y), nil)
	return at, l.xv, l.yv
}

type seenPair struct {
	x, y uintptr
	typ  reflect.Type
}

type locator struct {
	seen map[seenPair]bool
	r    *renderer // renders map keys, to order them as a rendering does

	// The values at the first difference: the operands of the innermost
	// find that reported one, unless a slice or map named them first.
	xv, yv reflect.Value
	found  bool
}

// find reports whether x and y differ and, if they do, the path of their
// first difference below at.
func (l *locator) find(x, y reflect.Value, at *step) (*step, bool) {
	s, d := l.findIn(x, y, at)
	if d && !l.found {
		l.xv, l.yv, l.found = x, y, true
	}
	return s, d
}

// there records the values at a difference the caller located itself.
func (l *locator) there(x, y reflect.Value) {
	l.xv, l.yv, l.found = x, y, true
}

// elementAt is v's element i, or the zero Value past its end.
func elementAt(v reflect.Value, i int) reflect.Value {
	if i < v.Len() {
		return v.Index(i)
	}
	return reflect.Value{}
}

func (l *locator) findIn(x, y reflect.Value, at *step) (*step, bool) {
	if !x.IsValid() || !y.IsValid() {
		return at, x.IsValid() != y.IsValid()
	}
	if x.Type() != y.Type() {
		return at, true
	}
	switch x.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if !x.IsNil() && !y.IsNil() {
			k := seenPair{x.Pointer(), y.Pointer(), x.Type()}
			if l.seen[k] {
				return at, false // on a cycle: equal unless shown otherwise
			}
			l.seen[k] = true
		}
	}
	switch x.Kind() {
	case reflect.Array:
		return l.elements(x, y, x.Len(), at)
	case reflect.Slice:
		if x.IsNil() != y.IsNil() {
			return at, true
		}
		if x.Type().Elem().Kind() == reflect.Uint8 {
			return byteDifference(x.Bytes(), y.Bytes(), at)
		}
		if x.Len() == y.Len() && x.Pointer() == y.Pointer() {
			return at, false
		}
		if s, d := l.elements(x, y, min(x.Len(), y.Len()), at); d {
			return s, true
		}
		if x.Len() != y.Len() {
			n := min(x.Len(), y.Len())
			l.there(elementAt(x, n), elementAt(y, n))
			return &step{up: at, index: n}, true
		}
		return at, false
	case reflect.Interface:
		if x.IsNil() || y.IsNil() {
			return at, x.IsNil() != y.IsNil()
		}
		return l.find(x.Elem(), y.Elem(), at)
	case reflect.Pointer:
		if x.Pointer() == y.Pointer() {
			return at, false
		}
		if x.IsNil() || y.IsNil() {
			return at, true
		}
		return l.find(x.Elem(), y.Elem(), at)
	case reflect.Struct:
		for i := range x.NumField() {
			if s, d := l.find(x.Field(i), y.Field(i), &step{up: at, field: x.Type().Field(i).Name}); d {
				return s, true
			}
		}
		return at, false
	case reflect.Map:
		return l.mapDifference(x, y, at)
	case reflect.Func:
		return at, !x.IsNil() || !y.IsNil()
	case reflect.String:
		return byteDifference([]byte(x.String()), []byte(y.String()), at)
	case reflect.Bool:
		return at, x.Bool() != y.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return at, x.Int() != y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return at, x.Uint() != y.Uint()
	case reflect.Float32, reflect.Float64:
		return at, x.Float() != y.Float()
	case reflect.Complex64, reflect.Complex128:
		return at, x.Complex() != y.Complex()
	case reflect.Chan, reflect.UnsafePointer:
		return at, x.Pointer() != y.Pointer()
	}
	return at, false
}

func (l *locator) elements(x, y reflect.Value, n int, at *step) (*step, bool) {
	for i := range n {
		if s, d := l.find(x.Index(i), y.Index(i), &step{up: at, index: i}); d {
			return s, true
		}
	}
	return at, false
}

// byteDifference names the first byte at which x and y differ.
func byteDifference(x, y []byte, at *step) (*step, bool) {
	if bytes.Equal(x, y) {
		return at, false
	}
	i := 0
	for i < len(x) && i < len(y) && x[i] == y[i] {
		i++
	}
	return &step{up: at, index: i}, true
}

// mapDifference finds the first key, in rendering order, that one map
// lacks or holds a different value under.
func (l *locator) mapDifference(x, y reflect.Value, at *step) (*step, bool) {
	if x.IsNil() != y.IsNil() {
		return at, true
	}
	if x.Len() == y.Len() && x.Pointer() == y.Pointer() {
		return at, false
	}
	type found struct {
		e      entry
		at     *step
		xv, yv reflect.Value // the values at the entry's first difference
	}
	var all []found
	keyStep := func(e *entry) *step { return &step{up: at, field: l.r.keyText(e, keyMax), key: true} }
	iter := x.MapRange()
	for iter.Next() {
		e := entry{key: iter.Key(), val: iter.Value()}
		yv := y.MapIndex(e.key)
		if !yv.IsValid() {
			all = append(all, found{e, nil, e.val, reflect.Value{}})
			continue
		}
		// Map order is random: each entry's values are taken apart, and
		// the first in rendering order are kept below.
		l.found = false
		if s, d := l.find(e.val, yv, nil); d {
			all = append(all, found{e, s, l.xv, l.yv})
		}
	}
	iter = y.MapRange()
	for iter.Next() {
		if !x.MapIndex(iter.Key()).IsValid() {
			all = append(all, found{entry{key: iter.Key(), val: iter.Value()}, nil, reflect.Value{}, iter.Value()})
		}
	}
	if len(all) == 0 {
		return at, false
	}
	native := l.r.nativeKey(x.Type().Key())
	order := func(e *entry) sortKey {
		if native {
			return keyOrder(e.key)
		}
		return sortKey{s: l.r.keyText(e, keyMax)}
	}
	first := 0
	for k := 1; k < len(all); k++ {
		if order(&all[k].e).compare(order(&all[first].e)) < 0 {
			first = k
		}
	}
	f := all[first]
	l.there(f.xv, f.yv)
	return graft(f.at, keyStep(&f.e)), true
}

// graft puts the path s, found from the top of a map's value, under at.
func graft(s, at *step) *step {
	if s == nil {
		return at
	}
	return &step{up: graft(s.up, at), field: s.field, key: s.key, index: s.index}
}
