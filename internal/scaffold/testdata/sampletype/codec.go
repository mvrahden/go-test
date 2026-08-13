package sampletype

// Encode/Decode: a natively-fuzzable inverse pair, both erroring — used to
// exercise the round-trip fuzz-scaffold path.
func Encode(s string) ([]byte, error) { return []byte(s), nil }
func Decode(b []byte) (string, error) { return string(b), nil }

// Render has no matching inverse anywhere in this package — used to
// exercise the no-inverse-pair crash-safety fuzz-scaffold fallback.
func Render(n int) string { return "" }

// Config/ApplyConfig: Config is a struct, so not natively fuzzable, but
// codec-fuzzable — used to exercise the codec-backed fuzz-scaffold path.
type Config struct{ Name string }

func ApplyConfig(c Config) string { return c.Name }

// ApplyOptions takes a map, which the codec emitter rejects (no canonical
// encoding) — used to exercise the not-fuzzable fuzz-scaffold fallback
// with the emitter's own rejection reason.
func ApplyOptions(opts map[string]string) string { return opts["name"] }
