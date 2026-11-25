// Package report prints with calc.
package report

import "mutmod/calc"

// Line is a score's grade.
func Line(score int) string { return calc.Grade(score) }
