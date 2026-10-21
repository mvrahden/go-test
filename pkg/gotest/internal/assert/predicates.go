package assert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"slices"
	"strings"
)

// The predicates decide what every assertion that asks counts as nil, empty,
// contained, panicking, a length or equal JSON. They live in the kernel so
// ring-0 tests check them without routing through the assertions built on them.

// DidPanic runs f and reports whether it panicked, with the recovered value.
func DidPanic(f func()) (recovered any, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			recovered = r
			panicked = true
		}
	}()
	f()
	return nil, false
}

// IncludesElement reports whether s contains element: a substring of a
// string kind, an element of a slice or array, a key of a map. guard names
// what the pair cannot be when s is none of those or element is a type s
// could not hold; it is empty when the question was answerable.
func IncludesElement(s, element any) (found bool, guard string) {
	if s == nil {
		return false, ""
	}
	rv := reflect.ValueOf(s)
	switch rv.Kind() {
	case reflect.String:
		ev := reflect.ValueOf(element)
		if !ev.IsValid() {
			return false, fmt.Sprintf("%T cannot hold a nil substring", s)
		}
		if ev.Kind() != reflect.String {
			return false, fmt.Sprintf("%T cannot hold a value of type %T%s", s, element, textHint(ev.Type(), rv.Type()))
		}
		return strings.Contains(rv.String(), ev.String()), ""
	case reflect.Slice, reflect.Array:
		want, guard := asType(element, rv.Type().Elem(), nilableElem, "element", "value")
		if guard != "" {
			return false, fmt.Sprintf("%T cannot hold %s", s, guard)
		}
		w := want.Interface()
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), w) {
				return true, ""
			}
		}
		return false, ""
	case reflect.Map:
		key, guard := asType(element, rv.Type().Key(), nilableKey, "key", "key")
		if guard != "" {
			return false, fmt.Sprintf("%T cannot hold %s", s, guard)
		}
		if !key.Comparable() {
			return false, fmt.Sprintf("%T cannot hold a key of type %T: it is not comparable", s, element)
		}
		return rv.MapIndex(key).IsValid(), ""
	}
	return false, fmt.Sprintf("object of type %T is not a string, slice, array, or map", s)
}

// The kinds whose zero value is nil, for elements and for map keys.
var (
	nilableElem = []reflect.Kind{reflect.Interface, reflect.Ptr, reflect.Chan, reflect.UnsafePointer, reflect.Slice, reflect.Map, reflect.Func}
	nilableKey  = []reflect.Kind{reflect.Interface, reflect.Ptr, reflect.Chan, reflect.UnsafePointer}
)

// asType returns element as a value of typ: an untyped nil is typ's nil
// when typ's kind is one of nilable. Otherwise guard names what element
// cannot be, in the words of the container: "a nil <nilNoun>", "a <typedNoun>
// of type T".
func asType(element any, typ reflect.Type, nilable []reflect.Kind, nilNoun, typedNoun string) (v reflect.Value, guard string) {
	ev := reflect.ValueOf(element)
	if !ev.IsValid() {
		if slices.Contains(nilable, typ.Kind()) {
			return reflect.Zero(typ), ""
		}
		return ev, "a nil " + nilNoun
	}
	if ev.Type().AssignableTo(typ) {
		return ev, ""
	}
	return ev, fmt.Sprintf("a %s of type %s%s", typedNoun, ev.Type(), convertHint(ev, typ))
}

// convertHint suggests converting a needle to typ when the conversion keeps
// its meaning: string kind to string kind, or a number that converts to typ
// and back unchanged, with its sign. 3.7 to int, 300 to int8 or -1 to uint
// gets no hint.
func convertHint(ev reflect.Value, to reflect.Type) string {
	from := ev.Type()
	if !from.ConvertibleTo(to) {
		return ""
	}
	keeps := from.Kind() == reflect.String && to.Kind() == reflect.String
	if isNumber(from.Kind()) && isNumber(to.Kind()) {
		cv := ev.Convert(to)
		keeps = cv.Convert(from).Equal(ev) && negative(cv) == negative(ev) // NaN never does
	}
	if keeps {
		return " (convert it to " + to.String() + ")"
	}
	return ""
}

// textHint suggests converting a rune, byte, or a slice of either to the
// string kind to; string(r) is the text the needle names.
func textHint(from, to reflect.Type) string {
	k := from.Kind()
	if k == reflect.Slice {
		k = from.Elem().Kind()
	}
	if k != reflect.Int32 && k != reflect.Uint8 {
		return ""
	}
	return " (convert it to " + to.String() + ")"
}

func negative(v reflect.Value) bool {
	switch {
	case v.CanInt():
		return v.Int() < 0
	case v.CanFloat():
		return v.Float() < 0
	}
	return false // unsigned, and complex, which converts only to complex
}

func isNumber(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Complex128
}

// IsNil reports whether object is nil, for the kinds that can be.
func IsNil(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}

// IsNilable reports whether object is of a kind that can be nil.
func IsNilable(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return true
	}
	return false
}

// IsEmptyable reports whether object is of a kind that can be empty.
func IsEmptyable(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array,
		reflect.Chan, reflect.String, reflect.Ptr:
		return true
	}
	return false
}

// IsEmpty reports whether a value is considered empty: nil, zero-length
// (slices, maps, arrays, channels, strings), or a pointer to an empty value.
func IsEmpty(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.Chan, reflect.String:
		return rv.Len() == 0
	case reflect.Ptr:
		if rv.IsNil() {
			return true
		}
		return IsEmpty(rv.Elem().Interface())
	}
	return false
}

// ReadAndRestore reads r to its end and, when r can seek, rewinds it so a
// later reader sees the same bytes.
func ReadAndRestore(r io.Reader) ([]byte, error) {
	if seeker, ok := r.(io.Seeker); ok {
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err == nil {
			defer func() { _, _ = seeker.Seek(pos, io.SeekStart) }()
		}
	}
	return io.ReadAll(r)
}

// NormalizeJSON converts a value to a JSON-comparable form.
// Accepts string, []byte, json.RawMessage, io.Reader, or any JSON-marshalable value.
func NormalizeJSON(v any) (any, error) {
	var raw []byte
	switch val := v.(type) {
	case string:
		raw = []byte(val)
	case []byte:
		raw = val
	case json.RawMessage:
		raw = []byte(val)
	case io.Reader:
		b, err := ReadAndRestore(val)
		if err != nil {
			return nil, fmt.Errorf("failed to read value: %w", err)
		}
		raw = b
	default:
		var err error
		raw, err = json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}
	}
	// Numbers stay as written: decoding them into float64 would make
	// 9007199254740993 and 9007199254740992 the same JSON.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	// Anything but whitespace after the value, a stray } or ] included, is an error.
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("failed to unmarshal JSON: trailing data after the value")
	}
	return out, nil
}

// Length reports the length of a slice, array, map, channel or string.
// ok is false for any other kind, including untyped nil and pointers.
func Length(object any) (n int, ok bool) {
	if object == nil {
		return 0, false
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.String:
		return rv.Len(), true
	}
	return 0, false
}

// JSONEqual reports whether two values from NormalizeJSON are the same JSON.
// Numbers are compared as the exact values they spell, so 1 equals 1.0 and
// 1e2 equals 100, while integers beyond float64's exact range stay apart.
func JSONEqual(expected, actual any) bool {
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok || len(a) != len(e) {
			return false
		}
		for k, ev := range e {
			av, ok := a[k]
			if !ok || !JSONEqual(ev, av) {
				return false
			}
		}
		return true
	case []any:
		a, ok := actual.([]any)
		if !ok || len(a) != len(e) {
			return false
		}
		for i := range e {
			if !JSONEqual(e[i], a[i]) {
				return false
			}
		}
		return true
	case json.Number:
		a, ok := actual.(json.Number)
		if !ok {
			return false
		}
		en, eok := parseDecimal(string(e))
		an, aok := parseDecimal(string(a))
		if !eok || !aok {
			return e == a
		}
		return en.equal(an)
	}
	return reflect.DeepEqual(expected, actual)
}

// decimal is a number as sign, significant digits and exponent:
// digits × 10^exp, with no leading or trailing zero in digits. Zero has no
// digits and no sign, so -0 is 0.
type decimal struct {
	neg    bool
	digits string
	exp    *big.Int
}

func (d decimal) equal(o decimal) bool {
	return d.neg == o.neg && d.digits == o.digits && d.exp.Cmp(o.exp) == 0
}

// parseDecimal reads a JSON number exactly. Its cost is linear in the
// spelling, whatever exponent it names.
func parseDecimal(s string) (decimal, bool) {
	var d decimal
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		d.neg, s = true, rest
	}
	mant, expText := s, "0"
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant, expText = s[:i], strings.TrimPrefix(s[i+1:], "+")
	}
	intPart, frac, _ := strings.Cut(mant, ".")
	if intPart == "" || !allDigits(intPart) || !allDigits(frac) {
		return d, false
	}
	exp, ok := new(big.Int).SetString(expText, 10)
	if !ok {
		return d, false
	}
	digits := strings.TrimLeft(intPart+frac, "0")
	exp.Sub(exp, big.NewInt(int64(len(frac))))
	trimmed := strings.TrimRight(digits, "0")
	exp.Add(exp, big.NewInt(int64(len(digits)-len(trimmed))))
	if trimmed == "" {
		return decimal{exp: new(big.Int)}, true
	}
	d.digits, d.exp = trimmed, exp
	return d, true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
