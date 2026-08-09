package brokenxtest

import "testing"

func TestFour(t *testing.T) {
	if Four() != 4 {
		t.Fatal("want 4")
	}
}
