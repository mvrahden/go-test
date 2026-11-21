// A fixture module whose second method starts a goroutine that panics: the
// process dies with the method running, so the stream holds its start and no
// verdict. It lives outside the fixture corpus so the corpus keeps its
// asserted shape.
module gotest.cutshort

go 1.26.0

replace github.com/mvrahden/go-test => ../../..

require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000
