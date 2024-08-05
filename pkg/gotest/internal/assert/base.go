package assert

import (
	"fmt"
	"reflect"
)

type formatF func(format string, args ...any)

func New(fmtFn formatF) *BaseAsserter {
	return &BaseAsserter{asserterFunc: defaultAsserterFunc(fmtFn)}
}

func defaultAsserterFunc(fmtFn formatF) func(func() error) {
	return func(testFn func() error) {
		err := testFn()
		if err != nil {
			fmtFn(err.Error())
		}
	}
}

type BaseAsserter struct {
	asserterFunc func(func() error)
}

func (b *BaseAsserter) Assert(v any) *BaseAssertionContext {
	return &BaseAssertionContext{v: v, asserterFunc: b.asserterFunc}
}

type BaseAssertionContext struct {
	v            any
	asserterFunc func(func() error)
}

func (b *BaseAssertionContext) IsTrue() {
	b.asserterFunc(func() error {
		v, ok := b.v.(bool)
		if !ok || !v {
			return fmt.Errorf("%v is not true", b.v)
		}
		return nil
	})
}
func (b *BaseAssertionContext) IsFalse() {
	b.asserterFunc(func() error {
		v, ok := b.v.(bool)
		if !ok || v {
			return fmt.Errorf("%v is not false", b.v)
		}
		return nil
	})
}

func (b *BaseAssertionContext) IsEqualTo(v any) {
	b.asserterFunc(func() error {
		ok := reflect.DeepEqual(b.v, v)
		if !ok {
			fmtStr := applyFmtTokens("{{FMT}} is not equal to {{FMT}}", b.v, v)
			return fmt.Errorf(fmtStr, b.v, v)
		}
		return nil
	})
}
func (b *BaseAssertionContext) IsZero() {
	b.asserterFunc(func() error {
		rVal := reflect.ValueOf(b.v)
		if !rVal.IsValid() {
			fmtStr := applyFmtTokens("%v can not be zero", b.v)
			return fmt.Errorf(fmtStr, b.v)
		}
		ok := rVal.IsZero()
		if !ok {
			return fmt.Errorf("%v is not zero", b.v)
		}
		return nil
	})
}
func (b *BaseAssertionContext) IsEmpty() {
	b.asserterFunc(func() error {
		cmpFn := func(actual int) error {
			if actual != 0 {
				return fmt.Errorf("is not empty (actual length = %d)", actual)
			}
			return nil
		}
		rVal := unwrapPtr(reflect.ValueOf(b.v))

		switch k := rVal.Kind(); k {
		case reflect.String, reflect.Map, reflect.Array, reflect.Slice, reflect.Chan:
			return cmpFn(rVal.Len())
		default:
			return fmt.Errorf("value of type <%s> can not be empty", k.String())
		}
	})
}
func (b *BaseAssertionContext) HasLength(l int) {
	b.asserterFunc(func() error {
		cmpFn := func(actual, expected int) error {
			if actual != expected {
				return fmt.Errorf("is not of length %d (actual length = %d)", expected, actual)
			}
			return nil
		}
		rVal := unwrapPtr(reflect.ValueOf(b.v))

		switch k := rVal.Kind(); k {
		case reflect.String, reflect.Map, reflect.Array, reflect.Slice, reflect.Chan:
			return cmpFn(rVal.Len(), l)
		default:
			return fmt.Errorf("value of type <%s> does not have a length", k.String())
		}
	})
}
func (b *BaseAssertionContext) HasCapacity(c int) {
	b.asserterFunc(func() error {
		cmpFn := func(actual, expected int) error {
			if actual != expected {
				return fmt.Errorf("is not of capacity %d (actual capacity = %d)", expected, actual)
			}
			return nil
		}
		rVal := reflect.ValueOf(b.v)
		if rVal.Kind() == reflect.Ptr && !rVal.IsNil() {
			rVal = rVal.Elem()
		}

		switch k := rVal.Kind(); k {
		case reflect.Array, reflect.Slice, reflect.Chan:
			return cmpFn(rVal.Cap(), c)
		default:
			return fmt.Errorf("value of type <%s> does not have a capacity", k.String())
		}
	})
}
func (b *BaseAssertionContext) Contains(v any) {
	b.asserterFunc(func() error {
		rVal := reflect.ValueOf(b.v)
		vVal := reflect.ValueOf(v)
		// vK := vVal.Kind()

		switch k := rVal.Kind(); k {
		case reflect.String:

		case reflect.Array, reflect.Slice:
			for i := range rVal.Len() {
				ok := reflect.DeepEqual(rVal.Index(i), vVal)
				if ok {
					return nil
				}
			}
		default:
			return fmt.Errorf("value of type <%s> does not have a capacity", k.String())
		}
		return fmt.Errorf("")
	})
}
func (b *BaseAssertionContext) ContainsAll(v ...any) { panic("not implemented yet") }
func (b *BaseAssertionContext) ContainsAny(v ...any) { panic("not implemented yet") }
