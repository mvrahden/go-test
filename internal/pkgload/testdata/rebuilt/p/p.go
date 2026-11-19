// Package p is imported by q, which p's external test imports: go
// rebuilds q against p's test variant.
package p

// Item is what q makes.
type Item struct{ Name string }
