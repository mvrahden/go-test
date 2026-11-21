package inpkg

import "testing"

func TestShout(t *testing.T) {
	if got := Shout("a"); got != "A!" {
		t.Fatal(got)
	}
}
