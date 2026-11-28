// Package pay computes fees.
package pay

// Fee is the fee of an amount.
func Fee(n int) int {
	if n > 100 {
		return n / 10
	}
	return 1
}
