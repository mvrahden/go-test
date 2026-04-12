package gotest

import "testing"

func Test_resolveFocus_no_focused_items_returns_unchanged(t *testing.T) {
	tests := []testEntry{{name: "a"}, {name: "b"}}
	descs := []describeEntry{{name: "c"}}
	rt, rd := resolveFocus(tests, descs)
	if len(rt) != 2 || len(rd) != 1 {
		t.Fatalf("expected 2 tests and 1 describe, got %d and %d", len(rt), len(rd))
	}
	if rt[0].excluded || rt[1].excluded || rd[0].excluded {
		t.Fatal("nothing should be excluded when no focus exists")
	}
}

func Test_resolveFocus_focused_test_excludes_others(t *testing.T) {
	tests := []testEntry{{name: "a"}, {name: "b", focused: true}, {name: "c"}}
	rt, _ := resolveFocus(tests, nil)
	if !rt[0].excluded {
		t.Fatal("non-focused test 'a' should be excluded")
	}
	if rt[1].excluded {
		t.Fatal("focused test 'b' should NOT be excluded")
	}
	if !rt[2].excluded {
		t.Fatal("non-focused test 'c' should be excluded")
	}
}

func Test_resolveFocus_focused_describe_excludes_other_describes(t *testing.T) {
	descs := []describeEntry{{name: "a"}, {name: "b", focused: true}}
	_, rd := resolveFocus(nil, descs)
	if !rd[0].excluded {
		t.Fatal("non-focused describe 'a' should be excluded")
	}
	if rd[1].excluded {
		t.Fatal("focused describe 'b' should NOT be excluded")
	}
}

func Test_resolveFocus_focused_test_also_excludes_non_focused_describes(t *testing.T) {
	tests := []testEntry{{name: "a", focused: true}}
	descs := []describeEntry{{name: "b"}}
	_, rd := resolveFocus(tests, descs)
	if !rd[0].excluded {
		t.Fatal("non-focused describe should be excluded when a test is focused")
	}
}

func Test_resolveFocus_excluded_stays_excluded(t *testing.T) {
	tests := []testEntry{{name: "a", excluded: true}, {name: "b"}}
	rt, _ := resolveFocus(tests, nil)
	if !rt[0].excluded {
		t.Fatal("explicitly excluded test should remain excluded")
	}
}

func Test_resolveFocus_excluded_items_without_focus(t *testing.T) {
	tests := []testEntry{{name: "a"}, {name: "b", excluded: true}}
	rt, _ := resolveFocus(tests, nil)
	if rt[0].excluded {
		t.Fatal("non-excluded test should not be excluded")
	}
	if !rt[1].excluded {
		t.Fatal("excluded test should stay excluded")
	}
}
