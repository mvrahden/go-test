package main

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestParseMinFlag(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		args   []string
		expect int
	}{
		{desc: "no flag", args: []string{"--debug"}, expect: 0},
		{desc: "equals syntax", args: []string{"--min=80"}, expect: 80},
		{desc: "space syntax", args: []string{"--min", "90"}, expect: 90},
		{desc: "empty args", args: nil, expect: 0},
		{desc: "invalid value", args: []string{"--min=abc"}, expect: 0},
		{desc: "min at end no value", args: []string{"--min"}, expect: 0},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotest.Equal(t, tc.expect, parseMinFlag(tc.args))
		})
	}
}
