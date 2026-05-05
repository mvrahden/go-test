package main

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestParseMinFlag(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		args      []string
		expect    int
		expectErr bool
	}{
		{desc: "no flag", args: []string{"--debug"}, expect: 0},
		{desc: "equals syntax", args: []string{"--min=80"}, expect: 80},
		{desc: "space syntax", args: []string{"--min", "90"}, expect: 90},
		{desc: "empty args", args: nil, expect: 0},
		{desc: "invalid value", args: []string{"--min=abc"}, expectErr: true},
		{desc: "min at end no value", args: []string{"--min"}, expect: 0},
		{desc: "negative value", args: []string{"--min=-5"}, expectErr: true},
		{desc: "over 100", args: []string{"--min=150"}, expectErr: true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := parseMinFlag(tc.args)
			if tc.expectErr {
				gotest.True(t, err != nil, "expected error")
			} else {
				gotest.NoError(t, err)
				gotest.Equal(t, tc.expect, got)
			}
		})
	}
}
