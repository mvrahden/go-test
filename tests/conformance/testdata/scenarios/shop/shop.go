// Package shop is what the scenarios' suites test.
package shop

// Total sums the prices.
func Total(prices ...int) int {
	sum := 0
	for _, p := range prices {
		sum += p
	}
	return sum
}
