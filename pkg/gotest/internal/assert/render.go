package assert

import (
	"cmp"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// renderMaxLines and renderMaxBytes cap a rendering; a tail line says
	// it was cut.
	renderMaxLines = 50_000
	renderMaxBytes = 4 << 20
	// maxIndent is the deepest level indented; deeper lines keep its
	// indent and name their level.
	maxIndent = 32
	// compactMax is the longest one-line rendering shown without a diff.
	compactMax = 80
	// keyMax is the most of a map key rendered, to sort by and to show.
	keyMax = 4 << 10
	// bytesPerRow is how many bytes a line of hex holds.
	bytesPerRow = 8
)

var (
	goStringerType = reflect.TypeFor[fmt.GoStringer]()
	bytesType      = reflect.TypeFor[[]byte]()
)

// Render renders v as an indented Go literal for diffing: one field,
// element or entry per line at every depth, pointers followed, map
// entries sorted, unexported fields included and no addresses.
func Render(v any) string {
	return render(v, false, false).text()
}

// rendering is a value rendered one field, element or entry per line, each
// line with the path of the value it shows.
type rendering struct {
	lines []string
	paths []*step
	cut   string // what the rendering was cut at, or ""
}

func (r *rendering) text() string { return strings.Join(r.lines, "\n") }

func (r *rendering) equal(o *rendering) bool { return slices.Equal(r.lines, o.lines) }

// render renders v; raw calls no GoString method and opens sync values,
// typed wraps a scalar in its type.
func render(v any, raw, typed bool) *rendering {
	out := &rendering{}
	if v == nil {
		out.lines, out.paths = []string{"<nil>"}, []*step{nil}
		return out
	}
	r := &renderer{onPath: map[visit]bool{}, raw: raw, out: out}
	r.tree(reflect.ValueOf(v), 0, nil, "", "", typed)
	if out.cut != "" {
		out.lines = append(out.lines, "… cut at "+out.cut)
		out.paths = append(out.paths, nil)
	}
	return out
}

// compact renders v on one line in %#v's shape, without addresses. ok is
// false when it exceeds limit bytes or holds text that spans lines.
func compact(v any, limit int, raw, typed bool) (string, bool) {
	if v == nil {
		return "<nil>", true
	}
	r := &renderer{onPath: map[visit]bool{}, raw: raw, budget: limit}
	r.flat(reflect.ValueOf(v), typed)
	if r.over || r.multiline {
		return "", false
	}
	return r.sb.String(), true
}

// step is one step of a path from the top of a value: a field, an index or
// a map key.
type step struct {
	up    *step
	field string // a field name, or a rendered map key when key is set
	key   bool
	index int  // an index, when field is ""
	row   bool // a row of hex bytes from index on
}

// segment is this step as Go spells it.
func (s *step) segment() string {
	switch {
	case s.key:
		return "[" + s.field + "]"
	case s.field != "":
		return "." + s.field
	}
	return "[" + strconv.Itoa(s.index) + "]"
}

// visit is a pointer, map or slice on the path being rendered.
type visit struct {
	ptr uintptr
	n   int
	typ reflect.Type
}

type renderer struct {
	onPath map[visit]bool
	raw    bool

	// tree mode
	out   *rendering
	bytes int
	depth int // of the last line

	// flat mode
	sb        strings.Builder
	budget    int // < 0: unbounded
	over      bool
	multiline bool
}

func (r *renderer) emit(depth int, at *step, s string) {
	if r.out.cut != "" {
		return
	}
	if len(r.out.lines) == renderMaxLines {
		r.out.cut = strconv.Itoa(renderMaxLines) + " lines"
		return
	}
	mark := ""
	if depth > maxIndent && depth != r.depth {
		mark = "‹depth " + strconv.Itoa(depth) + "› "
	}
	line := indentation(min(depth, maxIndent)) + mark + s
	if r.bytes+len(line)+1 > renderMaxBytes {
		r.out.cut = "4 MiB"
		return
	}
	r.bytes += len(line) + 1
	r.depth = depth
	r.out.lines = append(r.out.lines, line)
	r.out.paths = append(r.out.paths, at)
}

// done stops a walk whose output was cut or ran over its budget.
func (r *renderer) done() bool { return r.over || (r.out != nil && r.out.cut != "") }

const indents = "                                                                "

func indentation(n int) string { return indents[:2*n] }

// enter marks a reference on the path; false means it is already there.
// The caller deletes k from onPath when it leaves.
func (r *renderer) enter(rv reflect.Value) (k visit, ok bool) {
	k = visit{rv.Pointer(), 0, rv.Type()}
	if rv.Kind() == reflect.Slice {
		k.n = rv.Len()
	}
	if r.onPath[k] {
		return k, false
	}
	r.onPath[k] = true
	return k, true
}

// tree emits rv as lines at depth, the first after prefix, the last
// before suffix. typed wraps scalars in their type, as inside interfaces.
func (r *renderer) tree(rv reflect.Value, depth int, at *step, prefix, suffix string, typed bool) {
	if r.done() {
		return
	}
	if gs, ok := r.goString(rv); ok {
		lines := strings.Split(gs, "\n")
		for k, l := range lines {
			if k == 0 {
				l = prefix + l
			}
			if k == len(lines)-1 {
				l += suffix
			}
			r.emit(depth, at, l)
		}
		return
	}
	t := rv.Type()
	if r.opaque(t) {
		r.emit(depth, at, prefix+t.String()+"{…}"+suffix)
		return
	}
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			r.emit(depth, at, prefix+"("+t.String()+")(nil)"+suffix)
			return
		}
		k, ok := r.enter(rv)
		if !ok {
			r.emit(depth, at, prefix+"("+t.String()+")(<cycle>)"+suffix)
			return
		}
		defer delete(r.onPath, k)
		if e := rv.Elem(); isScalar(e.Kind()) && !r.hasGoString(e) {
			r.emit(depth, at, prefix+"&"+scalar(e, true)+suffix)
			return
		}
		r.tree(rv.Elem(), depth, at, prefix+"&", suffix, false)
	case reflect.Interface:
		if rv.IsNil() {
			r.emit(depth, at, prefix+t.String()+"(nil)"+suffix)
			return
		}
		r.tree(rv.Elem(), depth, at, prefix, suffix, true)
	case reflect.Struct:
		if rv.NumField() == 0 {
			r.emit(depth, at, prefix+t.String()+"{}"+suffix)
			return
		}
		r.emit(depth, at, prefix+t.String()+"{")
		for i := range rv.NumField() {
			name := t.Field(i).Name
			r.tree(rv.Field(i), depth+1, &step{up: at, field: name}, name+": ", ",", false)
		}
		r.emit(depth, at, "}"+suffix)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			r.emit(depth, at, prefix+typeName(t)+"(nil)"+suffix)
			return
		}
		if rv.Len() == 0 {
			r.emit(depth, at, prefix+typeName(t)+"{}"+suffix)
			return
		}
		if rv.Kind() == reflect.Slice {
			k, ok := r.enter(rv)
			if !ok {
				r.emit(depth, at, prefix+t.String()+"(<cycle>)"+suffix)
				return
			}
			defer delete(r.onPath, k)
		}
		if t.Elem().Kind() == reflect.Uint8 {
			r.bytesTree(rv, depth, at, prefix, suffix)
			return
		}
		r.emit(depth, at, prefix+t.String()+"{")
		for i := range rv.Len() {
			if r.done() {
				return
			}
			r.tree(rv.Index(i), depth+1, &step{up: at, index: i}, "", ",", false)
		}
		r.emit(depth, at, "}"+suffix)
	case reflect.Map:
		if rv.IsNil() {
			r.emit(depth, at, prefix+t.String()+"(nil)"+suffix)
			return
		}
		if rv.Len() == 0 {
			r.emit(depth, at, prefix+t.String()+"{}"+suffix)
			return
		}
		k, ok := r.enter(rv)
		if !ok {
			r.emit(depth, at, prefix+t.String()+"(<cycle>)"+suffix)
			return
		}
		defer delete(r.onPath, k)
		r.emit(depth, at, prefix+t.String()+"{")
		for _, e := range r.sortedEntries(rv, keyMax, renderMaxLines-len(r.out.lines)) {
			if r.done() {
				return
			}
			k := r.keyText(&e, keyMax)
			r.tree(e.val, depth+1, &step{up: at, field: k, key: true}, k+": ", ",", false)
		}
		r.emit(depth, at, "}"+suffix)
	case reflect.String:
		if rv.Len() > renderMaxBytes-r.bytes {
			r.out.cut = "4 MiB"
			return
		}
		r.textTree(rv.String(), depth, at, prefix, "", suffix, scalar(rv, typed))
	default:
		r.emit(depth, at, prefix+scalar(rv, typed)+suffix)
	}
}

// textTree emits s quoted, one line per line of text, between open and
// close; whole is how it reads on a single line.
func (r *renderer) textTree(s string, depth int, at *step, prefix, open, suffix, whole string) {
	segs := strings.SplitAfter(s, "\n")
	if segs[len(segs)-1] == "" {
		segs = segs[:len(segs)-1]
	}
	if len(segs) < 2 {
		r.emit(depth, at, prefix+whole+suffix)
		return
	}
	end := ""
	if open != "" {
		end = ")"
	}
	r.emit(depth, at, prefix+open+strconv.Quote(segs[0])+" +")
	for k, seg := range segs[1:] {
		if r.done() {
			return
		}
		tail := " +"
		if k == len(segs)-2 {
			tail = end + suffix
		}
		r.emit(depth+1, at, strconv.Quote(seg)+tail)
	}
}

// bytesTree emits a byte slice that is UTF-8 text as a converted string,
// split at its newlines, and any other bytes as rows of hex.
func (r *renderer) bytesTree(rv reflect.Value, depth int, at *step, prefix, suffix string) {
	t := rv.Type()
	if rv.Kind() == reflect.Slice && utf8.Valid(rv.Bytes()) {
		if rv.Len() > renderMaxBytes-r.bytes {
			r.out.cut = "4 MiB"
			return
		}
		s := string(rv.Bytes())
		r.textTree(s, depth, at, prefix, typeName(t)+"(", suffix, typeName(t)+"("+strconv.Quote(s)+")")
		return
	}
	r.emit(depth, at, prefix+typeName(t)+"{")
	var row strings.Builder
	for i := 0; i < rv.Len() && !r.done(); i += bytesPerRow {
		row.Reset()
		for k := i; k < min(i+bytesPerRow, rv.Len()); k++ {
			if k > i {
				row.WriteString(" ")
			}
			fmt.Fprintf(&row, "0x%02x,", rv.Index(k).Uint())
		}
		r.emit(depth+1, &step{up: at, index: i, row: true}, row.String())
	}
	r.emit(depth, at, "}"+suffix)
}

// typeName is t as Go spells it, []byte for []uint8.
func typeName(t reflect.Type) string {
	if t == bytesType {
		return "[]byte"
	}
	return t.String()
}

type entry struct {
	key, val reflect.Value
	text     string // the rendered key, or "" until rendered
	cut      bool   // whether text was cut
	vtext    *tieText
	order    sortKey
}

// tieText holds a value's rendering once it is needed to order its key,
// shared by every copy of the entry.
type tieText struct {
	s    string
	done bool
}

// sortKey orders keys of a kind nativeKey accepts: strings by s, the
// others by n.
type sortKey struct {
	s string
	n uint64
}

func keyOrder(v reflect.Value) sortKey {
	switch v.Kind() {
	case reflect.String:
		return sortKey{s: v.String()}
	case reflect.Bool:
		if v.Bool() {
			return sortKey{n: 1}
		}
		return sortKey{}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return sortKey{n: uint64(v.Int()) ^ 1<<63} //nolint:gosec // G115: flips the sign bit to order as unsigned
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		bits := math.Float64bits(f)
		switch {
		case math.IsNaN(f):
			return sortKey{n: math.MaxUint64} // NaN last
		case bits>>63 == 1:
			return sortKey{n: ^bits}
		}
		return sortKey{n: bits | 1<<63}
	}
	return sortKey{n: v.Uint()}
}

func (k sortKey) compare(o sortKey) int {
	if c := strings.Compare(k.s, o.s); c != 0 {
		return c
	}
	return cmp.Compare(k.n, o.n)
}

// nativeKey reports whether keys of t sort by their value rather than by
// their rendering.
func (r *renderer) nativeKey(t reflect.Type) bool {
	if !r.raw && t.Implements(goStringerType) {
		return false
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// sortedEntries returns rv's first n entries ordered by key: numbers,
// strings and bools by value (NaN last), other keys by their first limit
// bytes of rendering; keys that tie are ordered by their values'
// renderings.
func (r *renderer) sortedEntries(rv reflect.Value, limit, n int) []entry {
	out := make([]entry, 0, rv.Len())
	iter := rv.MapRange()
	native := r.nativeKey(rv.Type().Key())
	// Only floats (NaN) and rendered keys can tie.
	ties := !native || rv.Type().Key().Kind() == reflect.Float32 || rv.Type().Key().Kind() == reflect.Float64
	for iter.Next() {
		e := entry{key: iter.Key(), val: iter.Value()}
		if ties {
			e.vtext = &tieText{}
		}
		if native {
			e.order = keyOrder(e.key)
		} else {
			e.text, e.cut = r.flatString(e.key, limit)
			e.order.s = e.text
		}
		out = append(out, e)
	}
	order := func(a, b entry) int {
		if c := a.order.compare(b.order); c != 0 {
			return c
		}
		return strings.Compare(r.valueText(&a), r.valueText(&b))
	}
	if len(out) > n {
		selectFirst(out, n, order)
		out = out[:n]
	}
	slices.SortFunc(out, order)
	return out
}

// selectFirst reorders s so that its first n elements are its n least.
func selectFirst[E any](s []E, n int, cmp func(a, b E) int) {
	lo, hi := 0, len(s)
	for hi-lo > 1 {
		m := lo + (hi-lo)/2
		if cmp(s[m], s[lo]) < 0 {
			s[m], s[lo] = s[lo], s[m]
		}
		if cmp(s[hi-1], s[lo]) < 0 {
			s[hi-1], s[lo] = s[lo], s[hi-1]
		}
		if cmp(s[hi-1], s[m]) < 0 {
			s[hi-1], s[m] = s[m], s[hi-1]
		}
		pivot := s[m]
		// [lo,lt) < pivot, [lt,gt) == pivot, [gt,hi) > pivot
		lt, i, gt := lo, lo, hi
		for i < gt {
			switch c := cmp(s[i], pivot); {
			case c < 0:
				s[lt], s[i] = s[i], s[lt]
				lt, i = lt+1, i+1
			case c > 0:
				gt--
				s[i], s[gt] = s[gt], s[i]
			default:
				i++
			}
		}
		switch {
		case n < lt:
			hi = lt
		case n <= gt:
			return
		default:
			lo = gt
		}
	}
}

// tieMax is the most of a value rendered to order keys that tie.
const tieMax = 64 << 10

func (r *renderer) valueText(e *entry) string {
	if e.vtext == nil {
		e.vtext = &tieText{}
	}
	if !e.vtext.done {
		e.vtext.s, _ = r.flatString(e.val, tieMax)
		e.vtext.done = true
	}
	return e.vtext.s
}

func (r *renderer) keyText(e *entry, limit int) string {
	if e.text == "" {
		e.text, e.cut = r.flatString(e.key, limit)
	}
	return e.text
}

// flatString is v on one line, cut after limit bytes with a "…", and
// whether it was cut; any newline a GoString puts in it is escaped.
func (r *renderer) flatString(v reflect.Value, limit int) (string, bool) {
	fr := &renderer{onPath: r.onPath, raw: r.raw, budget: limit}
	fr.flat(v, false)
	s := fr.sb.String()
	if fr.over {
		s = cutBytes(s, limit) + "…"
	}
	return strings.ReplaceAll(s, "\n", `\n`), fr.over
}

// cutBytes is s's longest prefix of at most n bytes that ends on a rune.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (r *renderer) write(s string) {
	if r.over {
		return
	}
	r.sb.WriteString(s)
	if r.budget >= 0 && r.sb.Len() > r.budget {
		r.over = true
	}
}

// flat writes rv on one line, in %#v's shape.
func (r *renderer) flat(rv reflect.Value, typed bool) {
	if r.over {
		return
	}
	if gs, ok := r.goString(rv); ok {
		r.multiline = r.multiline || strings.Contains(gs, "\n")
		r.write(gs)
		return
	}
	t := rv.Type()
	if r.opaque(t) {
		r.write(t.String() + "{…}")
		return
	}
	// Every entry takes at least one byte: too many cannot fit. A string
	// that cannot fit is quoted only as far as it can.
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		if r.budget >= 0 && rv.Len() > r.budget {
			r.over = true
			return
		}
	case reflect.String:
		if r.budget >= 0 && rv.Len() > r.budget {
			r.write(strconv.Quote(cutBytes(rv.String(), r.budget+1)))
			r.over = true
			return
		}
	}
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			r.write("(" + t.String() + ")(nil)")
			return
		}
		k, ok := r.enter(rv)
		if !ok {
			r.write("(" + t.String() + ")(<cycle>)")
			return
		}
		defer delete(r.onPath, k)
		if e := rv.Elem(); isScalar(e.Kind()) && !r.hasGoString(e) {
			r.write("&")
			r.scalarFlat(e, true)
			return
		}
		r.write("&")
		r.flat(rv.Elem(), false)
	case reflect.Interface:
		if rv.IsNil() {
			r.write(t.String() + "(nil)")
			return
		}
		r.flat(rv.Elem(), true)
	case reflect.Struct:
		r.write(t.String() + "{")
		for i := range rv.NumField() {
			if i > 0 {
				r.write(", ")
			}
			r.write(t.Field(i).Name + ":")
			r.flat(rv.Field(i), false)
		}
		r.write("}")
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			r.write(typeName(t) + "(nil)")
			return
		}
		if rv.Kind() == reflect.Slice && rv.Len() > 0 {
			k, ok := r.enter(rv)
			if !ok {
				r.write(t.String() + "(<cycle>)")
				return
			}
			defer delete(r.onPath, k)
		}
		if t.Elem().Kind() == reflect.Uint8 {
			r.bytesFlat(rv)
			return
		}
		r.write(t.String() + "{")
		for i := range rv.Len() {
			if i > 0 {
				r.write(", ")
			}
			r.flat(rv.Index(i), false)
		}
		r.write("}")
	case reflect.Map:
		if rv.IsNil() {
			r.write(t.String() + "(nil)")
			return
		}
		k, ok := r.enter(rv)
		if !ok {
			r.write(t.String() + "(<cycle>)")
			return
		}
		defer delete(r.onPath, k)
		limit := keyMax
		if r.budget >= 0 {
			limit = r.budget
		}
		r.write(t.String() + "{")
		for k, e := range r.sortedEntries(rv, limit, rv.Len()) {
			if r.over {
				return
			}
			if k > 0 {
				r.write(", ")
			}
			r.write(r.keyText(&e, limit) + ":")
			r.over = r.over || e.cut
			r.flat(e.val, false)
		}
		r.write("}")
	default:
		r.scalarFlat(rv, typed)
	}
}

// bytesFlat writes a byte slice that is UTF-8 text as a converted string,
// any other bytes in hex.
func (r *renderer) bytesFlat(rv reflect.Value) {
	t := rv.Type()
	if rv.Kind() == reflect.Slice && utf8.Valid(rv.Bytes()) {
		s := string(rv.Bytes())
		r.multiline = r.multiline || strings.Contains(strings.TrimSuffix(s, "\n"), "\n")
		r.write(typeName(t) + "(" + strconv.Quote(s) + ")")
		return
	}
	r.write(typeName(t) + "{")
	for i := range rv.Len() {
		if i > 0 {
			r.write(", ")
		}
		r.write(fmt.Sprintf("0x%02x", rv.Index(i).Uint()))
	}
	r.write("}")
}

func (r *renderer) scalarFlat(rv reflect.Value, typed bool) {
	if rv.Kind() == reflect.String && strings.Contains(strings.TrimSuffix(rv.String(), "\n"), "\n") {
		r.multiline = true
	}
	r.write(scalar(rv, typed))
}

func isScalar(k reflect.Kind) bool {
	switch k {
	case reflect.Pointer, reflect.Interface, reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		return false
	}
	return true
}

// scalar renders a value of a scalar kind as %#v does, reading it by kind
// so unexported fields render too; typed wraps it in its type. Unsigned
// integers are decimal, a uintptr hex.
func scalar(rv reflect.Value, typed bool) string {
	t := rv.Type()
	var s string
	switch rv.Kind() {
	case reflect.Bool:
		s = strconv.FormatBool(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s = strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s = strconv.FormatUint(rv.Uint(), 10)
	case reflect.Uintptr:
		s = "0x" + strconv.FormatUint(rv.Uint(), 16)
	case reflect.Float32:
		s = strconv.FormatFloat(rv.Float(), 'g', -1, 32)
	case reflect.Float64:
		s = strconv.FormatFloat(rv.Float(), 'g', -1, 64)
	case reflect.Complex64:
		s = strconv.FormatComplex(rv.Complex(), 'g', -1, 64)
	case reflect.Complex128:
		s = strconv.FormatComplex(rv.Complex(), 'g', -1, 128)
	case reflect.String:
		s = strconv.Quote(rv.String())
	case reflect.Func, reflect.Chan:
		// A func or channel is shown by type and presence: an address would
		// differ between equal values and mean nothing to a reader.
		if rv.IsNil() {
			return "(" + t.String() + ")(nil)"
		}
		return "(" + t.String() + ")(<non-nil>)"
	case reflect.UnsafePointer:
		if rv.IsNil() {
			return t.String() + "(nil)"
		}
		return t.String() + "(<non-nil>)"
	default:
		return t.String() + "{}"
	}
	if typed {
		return t.String() + "(" + s + ")"
	}
	return s
}

// opaque reports whether values of t render as their type alone: the
// types of package sync, whose fields are bookkeeping.
func (r *renderer) opaque(t reflect.Type) bool {
	return !r.raw && t.PkgPath() == "sync" && t.Kind() == reflect.Struct
}

func (r *renderer) hasGoString(rv reflect.Value) bool {
	return !r.raw && rv.CanInterface() && rv.Type().Implements(goStringerType)
}

// goString is rv's GoString, when it has one that can be called: not on a
// nil pointer, nor on a value reached through an unexported field.
func (r *renderer) goString(rv reflect.Value) (s string, ok bool) {
	if !rv.IsValid() || rv.Kind() == reflect.Interface || !r.hasGoString(rv) {
		return "", false
	}
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return "", false
	}
	defer func() {
		if recover() != nil {
			s, ok = "", false
		}
	}()
	return rv.Interface().(fmt.GoStringer).GoString(), true
}
