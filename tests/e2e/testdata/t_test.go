package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SimpleTestSuite struct{}

func (s *SimpleTestSuite) TestT(t *gotest.T) {
	t.It("should perform the test case", func(it *gotest.T) {
		it.T().Fatalf("failed for reasons")
	})
}

func (s *SimpleTestSuite) TestIsTrue(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.True(it, false)
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.True(it, true)
	})
}

func (s *SimpleTestSuite) TestIsFalse(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.False(it, true)
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.False(it, false)
	})
}

func (s *SimpleTestSuite) TestIsEqualTo(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.Equal(it, []string{"def"}, []string{"abc"})
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.Equal(it, []string{"abc"}, []string{"abc"})
	})
}

func (s *SimpleTestSuite) TestIsZero(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.Zero(it, "abc")
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.Zero(it, "")
	})
}

func (s *SimpleTestSuite) TestIsEmpty(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.Empty(it, []string{"abc"})
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.Empty(it, "")
		gotest.Empty(it, []string{})
	})
}

func (s *SimpleTestSuite) TestHasLength(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.Len(it, []string{"abc"}, 3)
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.Len(it, "abc", 3)
		gotest.Len(it, [1]string{}, 1)
	})
}

func (s *SimpleTestSuite) TestContains(t *gotest.T) {
	t.It("fails", func(it *gotest.T) {
		gotest.Contains(it, "hello", "xyz")
	})
	t.It("succeeds", func(it *gotest.T) {
		gotest.Contains(it, "hello", "ell")
		gotest.Contains(it, []string{"a", "b"}, "b")
	})
}

func (s *SimpleTestSuite) TestStdlibCompat(t *testing.T) {
	gotest.True(t, false)
}

func helperAssert(t *gotest.T) {
	gotest.True(t, false)
}

func (s *SimpleTestSuite) TestHelperTrace(t *gotest.T) {
	t.It("shows called from trace", func(it *gotest.T) {
		helperAssert(it)
	})
}
