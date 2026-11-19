// Package price is the README's library example, kept runnable so the README
// shows a test that passes under plain go test.
package price

// Total sums the item prices.
func Total(items []int) int {
	sum := 0
	for _, n := range items {
		sum += n
	}
	return sum
}
