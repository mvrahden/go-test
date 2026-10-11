package unidiff

// SetExpensiveFloor lowers the cost at which a split gives up on
// minimality, so small inputs reach that path; it returns the restore.
func SetExpensiveFloor(n int) (restore func()) {
	old := expensiveFloor
	expensiveFloor = n
	return func() { expensiveFloor = old }
}
