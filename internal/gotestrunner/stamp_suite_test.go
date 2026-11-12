package gotestrunner_test

import (
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StampTestSuite covers the object the runner adds to each test event of a
// suite process's -json block: where the test sits.
type StampTestSuite struct{}

func (s *StampTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// twinBlock is one process running a suite its internal and its external
// test package both declare.
var twinBlock = strings.Join([]string{
	`{"Time":"2026-11-10T10:00:00.1+01:00","Action":"start","Package":"p"}`,
	`{"Time":"2026-11-10T10:00:00.1+01:00","Action":"run","Package":"p","Test":"S"}`,
	`{"Time":"2026-11-10T10:00:00.2+01:00","Action":"run","Package":"p","Test":"S/TestCreate"}`,
	`{"Time":"2026-11-10T10:00:00.3+01:00","Action":"output","Package":"p","Test":"S/TestCreate","Output":"=== RUN   S/TestCreate\n"}`,
	`{"Time":"2026-11-10T10:00:00.4+01:00","Action":"pass","Package":"p","Test":"S/TestCreate","Elapsed":0.1}`,
	`{"Time":"2026-11-10T10:00:00.5+01:00","Action":"pass","Package":"p","Test":"S","Elapsed":0.4}`,
	`{"Time":"2026-11-10T10:00:00.6+01:00","Action":"run","Package":"p","Test":"S"}`,
	`{"Time":"2026-11-10T10:00:00.7+01:00","Action":"run","Package":"p","Test":"S/TestCreate#01"}`,
	`{"Time":"2026-11-10T10:00:00.8+01:00","Action":"run","Package":"p","Test":"S/TestCreate#01/when_x"}`,
	`{"Time":"2026-11-10T10:00:00.9+01:00","Action":"pass","Package":"p","Test":"S/TestCreate#01/when_x","Elapsed":0}`,
	`{"Time":"2026-11-10T10:00:01+01:00","Action":"pass","Package":"p","Test":"S/TestCreate#01","Elapsed":0.2}`,
	`{"Time":"2026-11-10T10:00:01.1+01:00","Action":"fail","Package":"p","Test":"S","Elapsed":0.5}`,
	`not json`,
	``,
}, "\n")

var stampObject = regexp.MustCompile(`,"Gotest":\{[^{}]*\}`)

func (s *StampTestSuite) TestEachTestEventSaysWhereItSits(t *gotest.T) {
	out := strings.Split(string(gotestrunner.ExportStampBlock([]byte(twinBlock))), "\n")
	in := strings.Split(twinBlock, "\n")
	gotest.Len(t, out, len(in))
	gotest.Equal(t, `{"Time":"2026-11-10T10:00:00.7+01:00","Action":"run","Package":"p","Test":"S/TestCreate#01","Gotest":{"Subtest":"TestCreate#01","Repeat":1,"Instance":2}}`, out[7])
	gotest.Equal(t, `{"Time":"2026-11-10T10:00:00.1+01:00","Action":"run","Package":"p","Test":"S","Gotest":{"Subtest":"S"}}`, out[1])
	gotest.Equal(t, `{"Time":"2026-11-10T10:00:00.6+01:00","Action":"run","Package":"p","Test":"S","Gotest":{"Subtest":"S","Instance":2}}`, out[6])
	gotest.Equal(t, `{"Time":"2026-11-10T10:00:00.8+01:00","Action":"run","Package":"p","Test":"S/TestCreate#01/when_x","Gotest":{"Subtest":"when_x","Instance":2}}`, out[8])
	gotest.Equal(t, `{"Time":"2026-11-10T10:00:01.1+01:00","Action":"fail","Package":"p","Test":"S","Elapsed":0.5,"Gotest":{"Instance":2}}`, out[11])
	gotest.Equal(t, in[3], out[3]) // an output event of the first instance
	gotest.Equal(t, in[4], out[4])
}

func (s *StampTestSuite) TestEveryOtherByteIsKept(t *gotest.T) {
	out := string(gotestrunner.ExportStampBlock([]byte(twinBlock)))
	gotest.NotEqual(t, twinBlock, out)
	gotest.Equal(t, twinBlock, stampObject.ReplaceAllString(out, ""))
}

func (s *StampTestSuite) TestABlockWithoutTestEventsIsCopied(t *gotest.T) {
	block := "{\"Action\":\"output\",\"Package\":\"p\",\"Output\":\"ok\\n\"}\r\n{\"Action\":\"pass\",\"Package\":\"p\"}"
	gotest.Equal(t, block, string(gotestrunner.ExportStampBlock([]byte(block))))
}
