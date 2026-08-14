package gotest

// Codec is a decoder/encoder pair for one fuzz argument type that Go's
// fuzzing engine does not accept natively — a struct, or a named type over a
// native one. gotest generates one per such type a package fuzzes and
// attaches it to the *F the generated wrapper hands your Fuzz* method; Fuzz
// then reroutes to a native []byte target and decodes on the way in.
//
// Decode must be total: every byte string, including a short or empty one,
// has to yield a value. A decoder that can reject its input wastes a
// coverage-guided execution.
//
// You never construct a Codec. Its fields are exported only so generated
// code can build one as a composite literal; the mechanism is internal, and
// deliberately so — it can be replaced without a deprecation cycle.
type Codec[A any] struct {
	Decode func([]byte) A
	Encode func(A) []byte

	// Literal renders v as self-contained Go source, e.g. `Request{Name: "a"}`.
	// Generated code sets it only for types that have such a form; it is nil
	// otherwise, and every consumer falls back to raw corpus bytes.
	Literal func(A) string
}

// encodeAny lets F.Add turn a typed seed into the bytes the rerouted target
// expects without knowing A. v.(A) is a type assertion to a type parameter —
// a language construct, not the reflect package, and it inspects only the
// dynamic type, never the user's data.
//
// Being unexported, it also makes fuzzCodec unimplementable outside this
// package.
func (c Codec[A]) encodeAny(v any) ([]byte, bool) {
	a, ok := v.(A)
	if !ok {
		return nil, false
	}
	return c.Encode(a), true
}

// fuzzCodec is the erased form of Codec[A] that *F carries. One Fuzz call
// per target means the list is one or two entries, so the linear scans in
// Fuzz and Add are cheaper than any map — and the Fuzz-side scan happens
// once per fuzz target, not once per execution.
type fuzzCodec interface {
	encodeAny(v any) ([]byte, bool)
}
