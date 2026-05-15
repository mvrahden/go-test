package gotestrunner

import (
	"testing"
)

func TestExtractCoverProfile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		expect string
	}{
		{"empty", nil, ""},
		{"equals form", []string{"-v", "-coverprofile=cover.out"}, "cover.out"},
		{"space form", []string{"-coverprofile", "cover.out", "-v"}, "cover.out"},
		{"stops at -args", []string{"-args", "-coverprofile=cover.out"}, ""},
		{"no coverprofile", []string{"-v", "-count=1"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractCoverProfile(tc.flags)
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestStripCoverProfile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		expect []string
	}{
		{"empty", nil, nil},
		{"equals form", []string{"-v", "-coverprofile=cover.out", "-count=1"}, []string{"-v", "-count=1"}},
		{"space form", []string{"-coverprofile", "cover.out", "-v"}, []string{"-v"}},
		{"preserves -args passthrough", []string{"-v", "-args", "-coverprofile=x"}, []string{"-v", "-args", "-coverprofile=x"}},
		{"no coverprofile unchanged", []string{"-v", "-count=1"}, []string{"-v", "-count=1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := StripCoverProfile(tc.flags)
			if len(got) != len(tc.expect) {
				t.Fatalf("got %v, want %v", got, tc.expect)
			}
			for i := range got {
				if got[i] != tc.expect[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.expect[i])
				}
			}
		})
	}
}
