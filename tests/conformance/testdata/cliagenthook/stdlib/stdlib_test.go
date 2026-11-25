package stdlib_test

import "testing"

func TestAdds(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("1+1 is not 2")
	}
}
