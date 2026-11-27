package protocol_test

import (
	"encoding/json"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RecordModeTestSuite: the mode a record's verdict was taken under, and
// which runs it stands for.
type RecordModeTestSuite struct{}

func (s *RecordModeTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *RecordModeTestSuite) TestWhatAModeCovers(t *gotest.T) {
	plain, policy, short := protocol.VerdictMode{}, protocol.VerdictMode{Policy: true}, protocol.VerdictMode{Short: true}
	both := protocol.VerdictMode{Policy: true, Short: true}
	for sub, r := range gotest.Each(t, []struct {
		Desc    string
		verdict *protocol.VerdictMode
		run     protocol.VerdictMode
		want    bool
	}{
		{"no mode, a plain run", nil, plain, false},
		{"plain, a plain run", &plain, plain, true},
		{"policy, a plain run", &policy, plain, true},
		{"plain, a policy run", &plain, policy, false},
		{"policy and short, a policy and short run", &both, both, true},
		{"policy and short, a short run", &both, short, true},
		{"short, a plain run", &short, plain, false},
		{"plain, a short run", &plain, short, false},
	}) {
		gotest.Equal(sub, r.want, r.verdict.Covers(r.run))
	}
}

func (s *RecordModeTestSuite) TestTheModeInJSON(t *gotest.T) {
	t.It("writes the mode with both fields", func(it *gotest.T) {
		out, err := json.Marshal(protocol.PackageVerdict{Package: "m/a", Verdict: "pass", Mode: &protocol.VerdictMode{Policy: true}})
		gotest.NoError(it, err)
		gotest.Equal(it, `{"Package":"m/a","Verdict":"pass","Elapsed":0,"Mode":{"Policy":true,"Short":false}}`, string(out))
	})
	t.It("writes no mode for none", func(it *gotest.T) {
		out, err := json.Marshal(protocol.PackageVerdict{Package: "m/a", Verdict: "pass"})
		gotest.NoError(it, err)
		gotest.NotContains(it, string(out), `"Mode"`)
	})
	t.It("reads an entry without a mode as none", func(it *gotest.T) {
		var v protocol.PackageVerdict
		gotest.NoError(it, json.Unmarshal([]byte(`{"Package":"m/a","Verdict":"pass","Elapsed":0}`), &v))
		gotest.Zero(it, v.Mode)
	})
}
