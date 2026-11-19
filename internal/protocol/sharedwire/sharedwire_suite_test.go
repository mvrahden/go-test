package sharedwire_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/protocol/sharedwire"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedWireTestSuite: the shared fixture wire, byte for byte. A runner and
// a setup program from different releases meet on these lines.
type SharedWireTestSuite struct{}

func (s *SharedWireTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

var (
	stopCmd  = sharedwire.Command{Verb: sharedwire.VerbStop}
	forceCmd = sharedwire.Command{Verb: sharedwire.VerbForce}
	startCmd = sharedwire.Command{Verb: sharedwire.VerbStart, Keys: []string{"a/b.X", "a/b.Y"}, ID: 7}
)

func (s *SharedWireTestSuite) TestCommands(t *gotest.T) {
	t.When("the runner writes a command", func(w *gotest.T) {
		w.It("writes a stop or a force as its verb alone", func(it *gotest.T) {
			gotest.Equal(it, `{"verb":"stop"}`, string(stopCmd.Line()))
			gotest.Equal(it, `{"verb":"force"}`, string(forceCmd.Line()))
		})
		w.It("writes a window command with its keys and its id", func(it *gotest.T) {
			gotest.Equal(it, `{"verb":"start","keys":["a/b.X","a/b.Y"],"id":7}`, string(startCmd.Line()))
		})
	})
	t.When("the program reads a command", func(w *gotest.T) {
		w.It("reads back each command the runner writes", func(it *gotest.T) {
			for _, c := range []sharedwire.Command{stopCmd, forceCmd, startCmd} {
				got, ok := sharedwire.ParseCommand(c.Line())
				gotest.True(it, ok)
				gotest.Equal(it, c, got)
			}
		})
		w.It("refuses a line that is not a command", func(it *gotest.T) {
			_, ok := sharedwire.ParseCommand([]byte("not json"))
			gotest.False(it, ok)
			_, ok = sharedwire.ParseCommand(nil)
			gotest.False(it, ok)
		})
	})
}

func (s *SharedWireTestSuite) TestLines(t *gotest.T) {
	t.When("a setup begins", func(w *gotest.T) {
		w.It("names the fixture and the budget its setup is held to", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"_setup","fixture":"a/b.X","budget":"2m0s"}`, string(sharedwire.SetupLine("a/b.X", 2*time.Minute)))
			gotest.Equal(it, `{"key":"_setup","fixture":"a/b.X","budget":"0s"}`, string(sharedwire.SetupLine("a/b.X", 0)))
		})
	})
	t.When("a fixture is up", func(w *gotest.T) {
		w.It("writes its state under its key, an empty object for none", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"a/b.X","state":{"Addr":"x"}}`, string(sharedwire.StateLine("a/b.X", []byte(`{"Addr":"x"}`))))
			gotest.Equal(it, `{"key":"a/b.X","state":{}}`, string(sharedwire.StateLine("a/b.X", nil)))
		})
	})
	t.When("the setup is done", func(w *gotest.T) {
		w.It("writes no error for a good setup and the failure for a failed one, with the teardown budget", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"_done","teardownBudget":"2m30s"}`, string(sharedwire.DoneLine("", 150*time.Second)))
			gotest.Equal(it, `{"key":"_done","error":"one or more shared fixtures failed","teardownBudget":"30s"}`,
				string(sharedwire.DoneLine(sharedwire.SetupFailed, 30*time.Second)))
		})
	})
	t.When("a command is acknowledged", func(w *gotest.T) {
		w.It("always carries an error field, empty when it went well", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"_cmd","id":3,"error":"","teardownBudget":"32s"}`, string(sharedwire.AckLine(3, "", 32*time.Second)))
		})
		w.It("escapes <, > and & in an error, as encoding/json does", func(it *gotest.T) {
			const lt = "\x5cu003c" // the six bytes of a JSON-escaped <
			gotest.Equal(it, `{"key":"_cmd","id":4,"error":"X: a `+lt+` b","teardownBudget":"32s"}`, string(sharedwire.AckLine(4, "X: a < b", 32*time.Second)))
		})
	})
	t.When("a teardown failed", func(w *gotest.T) {
		w.It("names the failures on a teardown line", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"_teardown","error":"X: teardown failed"}`, string(sharedwire.TeardownLine("X: teardown failed")))
		})
	})
	t.When("the runner reads a line", func(w *gotest.T) {
		w.It("reads back each field the program writes", func(it *gotest.T) {
			got, ok := sharedwire.ParseLine(sharedwire.AckLine(3, "", 32*time.Second))
			gotest.True(it, ok)
			gotest.Equal(it, sharedwire.Line{Key: "_cmd", ID: 3, TeardownBudget: "32s"}, got)
			got, ok = sharedwire.ParseLine(sharedwire.SetupLine("a/b.X", 2*time.Minute))
			gotest.True(it, ok)
			gotest.Equal(it, sharedwire.Line{Key: "_setup", Fixture: "a/b.X", Budget: "2m0s"}, got)
			got, ok = sharedwire.ParseLine(sharedwire.StateLine("a/b.X", []byte(`{"Addr":"x"}`)))
			gotest.True(it, ok)
			gotest.Equal(it, sharedwire.Line{Key: "a/b.X", State: json.RawMessage(`{"Addr":"x"}`)}, got)
			got, ok = sharedwire.ParseLine(sharedwire.DoneLine(sharedwire.SetupFailed, 30*time.Second))
			gotest.True(it, ok)
			gotest.Equal(it, sharedwire.Line{Key: "_done", Error: sharedwire.SetupFailed, TeardownBudget: "30s"}, got)
			got, ok = sharedwire.ParseLine(sharedwire.TeardownLine("X: teardown failed"))
			gotest.True(it, ok)
			gotest.Equal(it, sharedwire.Line{Key: "_teardown", Error: "X: teardown failed"}, got)
		})
		w.It("refuses a line that is not JSON", func(it *gotest.T) {
			_, ok := sharedwire.ParseLine([]byte("setup says hello"))
			gotest.False(it, ok)
		})
		w.It("tells a sentinel, known or not, from a fixture's state", func(it *gotest.T) {
			gotest.True(it, sharedwire.Line{Key: "_done"}.Sentinel())
			gotest.True(it, sharedwire.Line{Key: "_future"}.Sentinel())
			gotest.False(it, sharedwire.Line{Key: "a/b.X"}.Sentinel())
			gotest.False(it, sharedwire.Line{Key: "a/b.my_X"}.Sentinel())
		})
	})
}

func (s *SharedWireTestSuite) TestState(t *gotest.T) {
	t.When("a state file is written and read", func(w *gotest.T) {
		w.It("holds each fixture's state under its key", func(it *gotest.T) {
			data, err := sharedwire.State{"k": json.RawMessage(`{"A":1}`)}.Encode()
			gotest.NoError(it, err)
			gotest.Equal(it, `{"k":{"A":1}}`, string(data))
			back, err := sharedwire.DecodeState(data)
			gotest.NoError(it, err)
			gotest.Equal(it, sharedwire.State{"k": json.RawMessage(`{"A":1}`)}, back)
		})
	})
	t.When("a state file is not a JSON object", func(w *gotest.T) {
		w.It("says the shared state could not be read", func(it *gotest.T) {
			_, err := sharedwire.DecodeState([]byte("["))
			gotest.ErrorContains(it, err, "unmarshal shared state")
		})
	})
}
