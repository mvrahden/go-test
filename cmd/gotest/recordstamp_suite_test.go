package main_test

import (
	"strings"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RecordStampTestSuite: what a run stamps on its record's packages, and
// which recorded verdicts --changed holds as current.
type RecordStampTestSuite struct{}

func (s *RecordStampTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *RecordStampTestSuite) TestStampingARecord(t *gotest.T) {
	t.It("gives each package the run's mode and marks the narrowed one", func(it *gotest.T) {
		rec := protocol.RunRecord{Packages: []protocol.PackageVerdict{{Package: "m/a", Verdict: "pass"}, {Package: "m/b", Verdict: "pass"}}}
		mode := protocol.VerdictMode{Policy: true, Short: true}
		main.ExportStampRecord(&rec, mode, func(pkg string) bool { return pkg == "m/b" }, false)
		a, b := rec.Packages[0], rec.Packages[1]
		gotest.NotZero(it, a.Mode)
		gotest.NotZero(it, b.Mode)
		gotest.Equal(it, mode, *a.Mode)
		gotest.Equal(it, mode, *b.Mode)
		gotest.NotSame(it, a.Mode, b.Mode)
		gotest.False(it, a.Narrowed)
		gotest.True(it, b.Narrowed)
	})
	// verdicts reads "pkg:verdict" words, a third field "n" marking one narrowed.
	verdicts := func(words string, mode *protocol.VerdictMode) []protocol.PackageVerdict {
		var out []protocol.PackageVerdict
		for _, w := range strings.Fields(words) {
			f := strings.Split(w, ":")
			out = append(out, protocol.PackageVerdict{Package: f[0], Verdict: f[1], Narrowed: len(f) > 2, Mode: mode})
		}
		return out
	}
	t.It("fails the packages a failure names and marks the passes of a failure no package explains", func(it *gotest.T) {
		for sub, r := range gotest.Each(it, []struct {
			Desc         string
			exit         int
			failed       string
			packages     string
			unattributed bool
			want         string
		}{
			{"a failure names a package", 1, "m/a", "m/a:pass m/b:pass", false, "m/a:fail m/b:pass"},
			{"exit 1 without a failure", 1, "", "m/a:pass m/b:pass m/c:skip", false, "m/a:pass:n m/b:pass:n m/c:skip"},
			{"exit 0 without a failure", 0, "", "m/a:pass", false, "m/a:pass"},
			{"an unattributed failure beside a failed package", 1, "m/a", "m/a:fail m/b:pass", true, "m/a:fail m/b:pass:n"},
			{"exit 0 with a failure listed", 0, "m/a", "m/a:pass", false, "m/a:fail"},
		}) {
			rec := protocol.RunRecord{Result: protocol.RunResult{ExitCode: r.exit}, Packages: verdicts(r.packages, nil)}
			if r.failed != "" {
				rec.Failed = []protocol.FailedUnit{{Package: r.failed, Test: "TestS/TestX"}}
			}
			main.ExportStampRecord(&rec, protocol.VerdictMode{}, nil, r.unattributed)
			gotest.Equal(sub, verdicts(r.want, &protocol.VerdictMode{}), rec.Packages)
		}
	})
}

func (s *RecordStampTestSuite) TestWhatChangedHoldsAsCurrent(t *gotest.T) {
	plain := &protocol.VerdictMode{}
	policy := &protocol.VerdictMode{Policy: true}
	short := &protocol.VerdictMode{Short: true}
	for sub, r := range gotest.Each(t, []struct {
		Desc            string
		entry           protocol.PackageVerdict
		hash            string
		mode            protocol.VerdictMode
		held, otherMode bool
	}{
		{"same hash, plain mode, a plain run", protocol.PackageVerdict{InputHash: "h", Mode: plain}, "h", protocol.VerdictMode{}, true, false},
		{"another hash", protocol.PackageVerdict{InputHash: "h", Mode: plain}, "g", protocol.VerdictMode{}, false, false},
		{"no hash recorded", protocol.PackageVerdict{Mode: plain}, "h", protocol.VerdictMode{}, false, false},
		{"narrowed", protocol.PackageVerdict{InputHash: "h", Mode: plain, Narrowed: true}, "h", protocol.VerdictMode{}, false, false},
		{"no mode", protocol.PackageVerdict{InputHash: "h"}, "h", protocol.VerdictMode{}, false, true},
		{"plain entry, a policy run", protocol.PackageVerdict{InputHash: "h", Mode: plain}, "h", protocol.VerdictMode{Policy: true}, false, true},
		{"short entry, a plain run", protocol.PackageVerdict{InputHash: "h", Mode: short}, "h", protocol.VerdictMode{}, false, true},
		{"policy entry, a plain run", protocol.PackageVerdict{InputHash: "h", Mode: policy}, "h", protocol.VerdictMode{}, true, false},
	}) {
		held, other := main.ExportHeldCurrent(r.entry, r.hash, r.mode)
		gotest.Equal(sub, []bool{r.held, r.otherMode}, []bool{held, other})
	}
}

func (s *RecordStampTestSuite) TestWhatNarrowsARun(t *gotest.T) {
	for sub, r := range gotest.Each(t, []struct {
		Desc                          string
		args, groups                  []string
		failedOnly, changedFailedOnly map[string]string
		focused                       bool
		declares                      map[string]bool
		a, b                          bool
	}{
		{"no flags", nil, nil, nil, nil, false, nil, false, false},
		{"-count=0", []string{"-count=0", "./..."}, nil, nil, nil, false, nil, true, true},
		{"a group", nil, []string{"fast"}, nil, nil, false, nil, true, true},
		{"a focus", nil, nil, nil, nil, true, nil, true, true},
		{"a focus beside a package declaring no test", nil, nil, nil, nil, true, map[string]bool{"m/a": true}, true, false},
		{"--failed naming a method", nil, nil, map[string]string{"m/a": "^TestS$/^TestX$"}, nil, false, nil, true, true},
		{"--failed rerunning a package whole", nil, nil, map[string]string{"m/a": ""}, nil, false, nil, false, true},
		{"--changed --failed", nil, nil, nil, map[string]string{"m/a": "", "m/b": "^TestS$/^TestX$"}, false, nil, false, true},
	}) {
		narrowed := main.ExportNarrowedBy(r.args, r.groups, r.failedOnly, r.changedFailedOnly, r.focused, r.declares)
		gotest.Equal(sub, []bool{r.a, r.b}, []bool{narrowed("m/a"), narrowed("m/b")})
	}
}
