package sampletype

// Encode/Decode: a natively-fuzzable inverse pair, both erroring — used to
// exercise the round-trip fuzz-scaffold path.
func Encode(s string) ([]byte, error) { return []byte(s), nil }
func Decode(b []byte) (string, error) { return string(b), nil }

// Render has no matching inverse anywhere in this package — used to
// exercise the no-inverse-pair crash-safety fuzz-scaffold fallback.
func Render(n int) string { return "" }

// Config/ApplyConfig: Config is a struct, not a natively fuzzable type —
// used to exercise the not-fuzzable fuzz-scaffold fallback.
type Config struct{ Name string }

func ApplyConfig(c Config) string { return c.Name }
