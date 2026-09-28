package gotestast

import (
	"errors"
	"go/token"
)

// SourceError is an error about one place in the source.
type SourceError struct {
	Pos token.Pos
	Err error
}

func (e *SourceError) Error() string { return e.Err.Error() }
func (e *SourceError) Unwrap() error { return e.Err }

// At gives err the position it is about. An error that carries one already
// keeps it: the innermost position is the most precise.
func At(pos token.Pos, err error) error {
	if err == nil || !pos.IsValid() || PosOf(err).IsValid() {
		return err
	}
	return &SourceError{Pos: pos, Err: err}
}

// PosOf returns the position err is about, token.NoPos when it has none.
func PosOf(err error) token.Pos {
	var se *SourceError
	if errors.As(err, &se) {
		return se.Pos
	}
	return token.NoPos
}
