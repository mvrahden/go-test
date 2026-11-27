// Package shop is what the audit's suites test.
package shop

// Add sums two numbers.
func Add(a, b int) int { return a + b }

// Clamp keeps n within 0 and limit.
func Clamp(n, limit int) int {
	if n < 0 {
		return 0
	}
	if n > limit {
		return limit
	}
	return n
}
