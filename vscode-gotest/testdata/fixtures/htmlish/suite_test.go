package htmlish

import "github.com/mvrahden/go-test/pkg/gotest"

// Failure output is rendered into a webview. A test whose message contains
// markup must appear as text, never as live HTML: the Spec View escapes it, and
// this fixture is what proves the escaping is still there.
type HTMLishTestSuite struct{}

func (s *HTMLishTestSuite) TestMarkupInOutput(t *gotest.T) {
	t.When("a failure message contains markup", func(t *gotest.T) {
		t.It("does not let a script tag through", func(t *gotest.T) {
			gotest.Equal(t, "safe", "<script>alert('xss')</script>")
		})
		t.It("does not let a closing tag break the panel", func(t *gotest.T) {
			gotest.Equal(t, "safe", "</div></body></html><img src=x onerror=alert(1)>")
		})
	})
}
