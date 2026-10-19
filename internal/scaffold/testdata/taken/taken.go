package taken

// TakenTestSuite is the name a scaffold of this file would declare.
const TakenTestSuite = "taken"

// Ledger has a method; its suite name is declared by an external test file.
type Ledger struct{}

// Post records an entry.
func (l *Ledger) Post() error { return nil }

// Journal is an interface; its contract suite name is declared by a test file.
type Journal interface {
	Close() error
}

// Settle takes one fuzzable parameter; its suite name is declared by a test file.
func Settle(n int) string { return "" }
