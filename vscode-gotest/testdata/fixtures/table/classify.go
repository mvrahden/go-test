package table

// Production source, not a test file. Coverage is measured against
// non-test source, so the corpus needs at least one package that has some —
// otherwise a coverage run produces an empty profile and proves nothing.
func classify(n int) string {
	switch {
	case n < 0:
		return "negative"
	case n == 0:
		return "zero"
	default:
		return "positive"
	}
}
