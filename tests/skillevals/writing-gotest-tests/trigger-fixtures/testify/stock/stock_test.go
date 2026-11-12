package stock

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestAdd(t *testing.T) {
	s := NewShelf()
	s.Add("apple", 3)
	s.Add("apple", 2)
	require.Equal(t, 5, s.Count("apple"))
}

type ShelfSuite struct {
	suite.Suite
	shelf *Shelf
}

func (s *ShelfSuite) SetupTest() { s.shelf = NewShelf() }

func (s *ShelfSuite) TestCountUnknown() {
	s.Require().Zero(s.shelf.Count("pear"))
}

func TestShelfSuite(t *testing.T) { suite.Run(t, new(ShelfSuite)) }
