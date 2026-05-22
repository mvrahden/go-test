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
