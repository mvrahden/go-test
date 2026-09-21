// A fixture module whose shared fixture refuses to tear down when
// GOTEST_TEARDOWN_FAIL is set: every suite passes and the run still fails, the
// one shape that used to reach the Test Explorer as a green tree. It lives
// outside the fixture corpus so the corpus keeps its asserted shape.
module gotest.teardownfail

go 1.26.0

replace github.com/mvrahden/go-test => ../../..

require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000
