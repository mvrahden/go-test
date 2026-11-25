// Package ledger has no test of its own.
package ledger

// Balance is what is left of a sum after a cost.
func Balance(sum, cost int) int {
	return sum - cost
}
