package gotestspec //nolint:stdlib-test

import (
	"strings"
	"testing"
)

func TestParseEvents_Empty(t *testing.T) {
	events, err := ParseEvents(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestParseEvents_BlankLinesSkipped(t *testing.T) {
	input := "\n\n" + `{"Action":"run","Package":"p","Test":"TestFoo"}` + "\n\n"
	events, err := ParseEvents(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Test != "TestFoo" {
		t.Errorf("test = %q, want TestFoo", events[0].Test)
	}
}

func TestParseEvents_MalformedJSONSkipped(t *testing.T) {
	input := `not json at all
{"Action":"run","Package":"p","Test":"TestA"}
{truncated
{"Action":"pass","Package":"p","Test":"TestA","Elapsed":0.01}`

	events, err := ParseEvents(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Action != ActionRun {
		t.Errorf("events[0].Action = %q, want run", events[0].Action)
	}
	if events[1].Action != ActionPass {
		t.Errorf("events[1].Action = %q, want pass", events[1].Action)
	}
}

func TestParseEvents_OutputCaptured(t *testing.T) {
	input := `{"Action":"output","Package":"p","Test":"TestFoo","Output":"hello world\n"}`
	events, err := ParseEvents(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Output != "hello world\n" {
		t.Errorf("output = %q", events[0].Output)
	}
}
