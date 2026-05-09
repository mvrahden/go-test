package simplesuite_test

import (
	"bytes"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SimpleTestSuite struct {
	buf *bytes.Buffer
}

func (s *SimpleTestSuite) BeforeAll(t *gotest.T) {
	s.buf = bytes.NewBufferString("")
}

func (s *SimpleTestSuite) BeforeEach(t *gotest.T) {
	_, err := s.buf.WriteString("+ 1 line\n")
	if err != nil {
		t.T().Errorf("failed writing to buffer: %s", err)
	}
}

func (s *SimpleTestSuite) AfterEach(t *gotest.T) {
	_, err := s.buf.WriteString("- 1 line\n")
	if err != nil {
		t.T().Errorf("failed writing to buffer: %s", err)
	}
}

func (s *SimpleTestSuite) AfterAll(t *gotest.T) {
	t.T().Fail()
}

func (s *SimpleTestSuite) TestSucceeds(t *gotest.T) {
	content := s.buf.String()
	if content != "+ 1 line\n" {
		t.T().Errorf("assertion failed: got: %q", content)
	}
}

func (s *SimpleTestSuite) TestFails(t *gotest.T) {
	content := s.buf.String()
	if content != "+ 1 line\n" {
		t.T().Errorf("assertion failed: got: %q", content)
	}
}
