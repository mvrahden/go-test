package main_test

import (
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
	plain := &protocol.VerdictMode{}
	pkg := func(name, verdict string, narrowed bool) protocol.PackageVerdict {
		return protocol.PackageVerdict{Package: name, Verdict: verdict, Narrowed: narrowed, Mode: plain}
	}
	t.It("fails the packages a failure names and marks the passes of a failure no package explains", func(it *gotest.T) {
		for sub, r := range gotest.Each(it, []struct {
			Desc         string
			record       protocol.RunRecord
			unattributed bool
			want         []protocol.PackageVerdict
		}{
			{
				"a failure names a package",
				protocol.RunRecord{Result: protocol.RunResult{ExitCode: 1}, Failed: []protocol.FailedUnit{{Package: "m/a", Test: "TestS/TestX"}}, Packages: []protocol.PackageVerdict{{Package: "m/a", Verdict: "pass"}, {Package: "m/b", Verdict: "pass"}}},
				false,
				[]protocol.PackageVerdict{pkg("m/a", "fail", false), pkg("m/b", "pass", false)},
			},
			{
				"exit 1 without a failure",
				protocol.RunRecord{Result: protocol.RunResult{ExitCode: 1}, Packages: []protocol.PackageVerdict{{Package: "m/a", Verdict: "pass"}, {Package: "m/b", Verdict: "pass"}, {Package: "m/c", Verdict: "skip"}}},
				false,
				[]protocol.PackageVerdict{pkg("m/a", "pass", true), pkg("m/b", "pass", true), pkg("m/c", "skip", false)},
			},
			{
				"exit 0 without a failure",
				protocol.RunRecord{Packages: []protocol.PackageVerdict{{Package: "m/a", Verdict: "pass"}}},
				false,
				[]protocol.PackageVerdict{pkg("m/a", "pass", false)},
			},
			{
				"an unattributed failure beside a failed package",
				protocol.RunRecord{Result: protocol.RunResult{ExitCode: 1}, Failed: []protocol.FailedUnit{{Package: "m/a", Test: "TestS/TestX"}}, Packages: []protocol.PackageVerdict{{Package: "m/a", Verdict: "fail"}, {Package: "m/b", Verdict: "pass"}}},
				true,
				[]protocol.PackageVerdict{pkg("m/a", "fail", false), pkg("m/b", "pass", true)},
			},
			{
				"exit 0 with a failure listed",
				protocol.RunRecord{Failed: []protocol.FailedUnit{{Package: "m/a", Test: "TestS/TestX"}}, Packages: []protocol.PackageVerdict{{Package: "m/a", Verdict: "pass"}}},
				false,
				[]protocol.PackageVerdict{pkg("m/a", "fail", false)},
			},
		}) {
			rec := r.record
			main.ExportStampRecord(&rec, protocol.VerdictMode{}, nil, r.unattributed)
			gotest.Equal(sub, r.want, rec.Packages)
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
		a, b                          bool
	}{
		{"no flags", nil, nil, nil, nil, false, false, false},
		{"-count=0", []string{"-count=0", "./..."}, nil, nil, nil, false, true, true},
		{"a group", nil, []string{"fast"}, nil, nil, false, true, true},
		{"a focus", nil, nil, nil, nil, true, true, true},
		{"--failed naming a method", nil, nil, map[string]string{"m/a": "^TestS$/^TestX$"}, nil, false, true, true},
		{"--failed rerunning a package whole", nil, nil, map[string]string{"m/a": ""}, nil, false, false, true},
		{"--changed --failed", nil, nil, nil, map[string]string{"m/a": "", "m/b": "^TestS$/^TestX$"}, false, false, true},
	}) {
		narrowed := main.ExportNarrowedBy(r.args, r.groups, r.failedOnly, r.changedFailedOnly, r.focused)
		gotest.Equal(sub, []bool{r.a, r.b}, []bool{narrowed("m/a"), narrowed("m/b")})
	}
}
