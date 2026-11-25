// Package checkout is what the evidence suites test.
package checkout

// Paid reports whether an amount settles a price.
func Paid(amount, price int) bool { return amount >= price }
