package gotest_test

import "fmt"

// mockT captures test failures without actually failing the test.
type mockT struct {
	failed  bool
	message string
}

func (m *mockT) Errorf(format string, args ...any) {
	m.failed = true
	m.message = fmt.Sprintf(format, args...)
}
func (m *mockT) Helper() {}
func (m *mockT) FailNow() {
	m.failed = true
}

func newMock() *mockT { return &mockT{} }

func sanitizeForTest(s string) string {
	for _, pair := range [][2]string{{"/", "_"}, {" ", "_"}, {":", "_"}} {
		result := ""
		for _, c := range s {
			if string(c) == pair[0] {
				result += pair[1]
			} else {
				result += string(c)
			}
		}
		s = result
	}
	return s
}
