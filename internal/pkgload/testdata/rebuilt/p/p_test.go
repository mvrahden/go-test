package p

import "testing"

func TestItem(t *testing.T) {
	if (Item{Name: "a"}).Name != "a" {
		t.Fatal()
	}
}
