package protocol_test

import (
	"fmt"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SwitchTestSuite covers how an on/off variable that takes no other value is read.
type SwitchTestSuite struct{}

func (s *SwitchTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const switchHint = " is neither on nor off: set it to 1 or true to switch it on, to 0 or false to switch it off, or unset it"

func (s *SwitchTestSuite) TestParseSwitch(t *gotest.T) {
	t.It("reads empty and the false spellings as off", func(it *gotest.T) {
		for _, v := range []string{"", "0", "f", "F", "false", "FALSE", "False"} {
			on, err := protocol.ParseSwitch("GOTEST_CI", v)
			gotest.NoError(it, err, "%q", v)
			gotest.False(it, on, "%q", v)
		}
	})
	t.It("reads the true spellings as on", func(it *gotest.T) {
		for _, v := range []string{"1", "t", "T", "true", "TRUE", "True"} {
			on, err := protocol.ParseSwitch("GOTEST_CI", v)
			gotest.NoError(it, err, "%q", v)
			gotest.True(it, on, "%q", v)
		}
	})
	t.It("refuses every other value, naming the variable and the value", func(it *gotest.T) {
		for _, v := range []string{"off", "no", "yes", "on", "2", "-1", "TrUe", " ", " 1", "1 ", "disabled"} {
			on, err := protocol.ParseSwitch("GOTEST_CI", v)
			gotest.False(it, on, "%q", v)
			gotest.Error(it, err, "%q", v)
			gotest.Equal(it, fmt.Sprintf("GOTEST_CI=%q", v)+switchHint, fmt.Sprint(err))
		}
	})
}

func (s *SwitchTestSuite) TestCheckSwitches(t *gotest.T) {
	env := func(m map[string]string) func(string) string {
		return func(name string) string { return m[name] }
	}
	t.It("passes when nothing is set", func(it *gotest.T) {
		gotest.NoError(it, protocol.CheckSwitches(env(nil)))
	})
	t.It("passes a true and a false spelling", func(it *gotest.T) {
		gotest.NoError(it, protocol.CheckSwitches(env(map[string]string{"GOTEST_CI": "1", "GOTEST_UPDATE_SNAPSHOTS": "0"})))
	})
	t.It("refuses a GOTEST_UPDATE_SNAPSHOTS that is no boolean", func(it *gotest.T) {
		err := protocol.CheckSwitches(env(map[string]string{"GOTEST_UPDATE_SNAPSHOTS": "off"}))
		gotest.Error(it, err)
		gotest.Equal(it, `GOTEST_UPDATE_SNAPSHOTS="off"`+switchHint, fmt.Sprint(err))
	})
	t.It("names GOTEST_CI first when both are refused", func(it *gotest.T) {
		err := protocol.CheckSwitches(env(map[string]string{"GOTEST_CI": "yes", "GOTEST_UPDATE_SNAPSHOTS": "off"}))
		gotest.Error(it, err)
		gotest.Equal(it, `GOTEST_CI="yes"`+switchHint, fmt.Sprint(err))
	})
	t.It("leaves the other variables alone", func(it *gotest.T) {
		gotest.NoError(it, protocol.CheckSwitches(env(map[string]string{"CI": "yes", "GOTEST_AGENT": "off", "GOTEST_LIVE": "maybe"})))
	})
}
