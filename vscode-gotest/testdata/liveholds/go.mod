// A fixture module whose two suites each pass a first method and then hold
// in a second until a file in LIVE_RELEASE_DIR releases them, so a test can
// watch results arrive while both suites still run.
module gotest.liveholds

go 1.26.0

replace github.com/mvrahden/go-test => ../../..

require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000
