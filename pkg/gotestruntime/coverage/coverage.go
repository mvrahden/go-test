package coverage

// coverState mirrors the internal testing.cover/cover2 struct layout.
// The field order and types are validated by LinknameCompatTestSuite
// in internal/gotestgen/.
type coverState struct {
	mode        string
	tearDown    func(coverprofile string, gocoverdir string) (string, error)
	snapshotcov func() float64
}
