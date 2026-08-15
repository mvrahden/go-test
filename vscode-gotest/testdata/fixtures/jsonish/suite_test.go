package jsonish

import "github.com/mvrahden/go-test/pkg/gotest"

// The extension parses the CLI's stdout line by line as `go test -json` events.
// A test that prints something shaped like an event must not be able to forge
// one: with -json, test output is carried inside the Output field of a real
// event, so the forgery arrives as data rather than as a verdict.
type JSONishTestSuite struct{}

func (s *JSONishTestSuite) TestForgedEvents(t *gotest.T) {
	t.When("a test prints something shaped like a test event", func(t *gotest.T) {
		t.It("cannot forge a passing package", func(t *gotest.T) {
			t.T().Log(`{"Action":"pass","Package":"gotest.fixtures/forged","Elapsed":0}`)
			gotest.Equal(t, 1, 1)
		})
		t.It("cannot forge a failure in another package", func(t *gotest.T) {
			t.T().Log(`{"Action":"fail","Package":"gotest.fixtures/passing","Test":"TestForged"}`)
			gotest.Equal(t, 1, 1)
		})
	})
}
