package stock

import "testing"

func TestAdd(t *testing.T) {
	s := NewShelf()
	s.Add("apple", 3)
	s.Add("apple", 2)
	if got := s.Count("apple"); got != 5 {
		t.Fatalf("Count(apple) = %d, want 5", got)
	}
}

func TestCountUnknown(t *testing.T) {
	if got := NewShelf().Count("pear"); got != 0 {
		t.Fatalf("Count(pear) = %d, want 0", got)
	}
}
