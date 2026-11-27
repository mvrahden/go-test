package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// VacuousCountsTestSuite covers how a text run reads the counts its suite
// process wrote: which leaves passed without an assertion.
type VacuousCountsTestSuite struct{}

func (s *VacuousCountsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *VacuousCountsTestSuite) TestTheLeavesAPassWithoutAnAssertionNames(t *gotest.T) {
	for t, r := range gotest.Each(t, []struct {
		Desc           string
		counts, stdout string
		want           []string
		unknown        bool
	}{
		{
			Desc:   "a zero count, a count, a declared means and a failed leaf",
			counts: `{"Test":"T/A","Assertions":0}` + "\n" + `{"Test":"T/B","Assertions":2}` + "\n" + `{"Test":"T/C","Assertions":0,"AssertsBy":true}` + "\n" + `{"Test":"T/D","Assertions":0,"Failed":true}` + "\n",
			stdout: "--- FAIL: T (0.00s)\n    --- FAIL: T/D (0.00s)\n",
			want:   []string{"T/A"},
		},
		{
			Desc:   "a line the process's end cut short",
			counts: `{"Test":"T/A","Assertions":0}` + "\n" + `{"Test":"T/B","Assert`,
			want:   []string{"T/A"},
		},
		{
			Desc:   "a leaf that panicked after it reported",
			counts: `{"Test":"T/A","Assertions":0}` + "\n",
			stdout: "--- FAIL: T (0.00s)\n    --- FAIL: T/A (0.00s)\npanic: boom\n",
		},
		{
			Desc:   "a leaf run twice under -count that panicked the second time",
			counts: `{"Test":"T/A","Assertions":0}` + "\n" + `{"Test":"T/A","Assertions":0}` + "\n",
			stdout: "    --- FAIL: T/A (0.00s)\npanic: boom\n",
			want:   []string{"T/A"},
		},
		{
			Desc:   "a leaf run twice under -count that failed the first time",
			counts: `{"Test":"T/A","Assertions":0,"Failed":true}` + "\n" + `{"Test":"T/A","Assertions":0}` + "\n",
			stdout: "    --- FAIL: T/A (0.00s)\n",
			want:   []string{"T/A"},
		},
		{
			Desc:    "a runtime that wrote nothing, which counts nothing",
			stdout:  "ok\n",
			unknown: true,
		},
	}) {
		vacuous, counted := gotestrunner.ExportVacuousInText(r.counts, r.stdout)
		gotest.Equal(t, r.want, vacuous)
		gotest.Equal(t, !r.unknown, counted)
	}
}
