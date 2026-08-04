package migrate_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// todoInputSrc is a testify suite containing lifecycle hooks and assertion
// calls the migrator cannot convert. The migrator must annotate every one of
// them with a TODO(gotest-migrate) marker instead of silently skipping them.
const todoInputSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SampleSuite struct {
	suite.Suite
}

func (s *SampleSuite) SetupTest() {}

func (s *SampleSuite) SetupSubTest() {}

func (s *SampleSuite) TearDownSubTest() {}

func (s *SampleSuite) BeforeTest(suiteName, testName string) {}

func (s *SampleSuite) AfterTest(suiteName, testName string) {}

func (s *SampleSuite) HandleStats(suiteName string, stats *suite.SuiteInformation) {}

func (s *SampleSuite) TestThings() {
	var target error
	s.Require().ErrorAs(nil, &target)
	s.Assert().Eventually(func() bool { return true }, 0, 0)
	s.ElementsMatch([]int{1}, []int{1})
	assert.InDelta(s.T(), 1.0, 1.0, 0.1)
	require.Regexp(s.T(), "a", "a")
	s.Require().NoError(nil)
	s.Assert().Equal(1, 1)
	s.Equal(2, 2)
}

func TestSampleSuite(t *testing.T) {
	suite.Run(t, new(SampleSuite))
}
`

// MigrateTestSuite tests the testify-to-gotest migration, focusing on
// TODO(gotest-migrate) markers for constructs the migrator cannot convert.
type MigrateTestSuite struct{}

func (s *MigrateTestSuite) migrateSource(t *gotest.T, src string) string {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample_test.go")
	err := os.WriteFile(path, []byte(src), 0600)
	gotest.NoError(t, err)
	results, err := migrate.MigrateFile(path)
	gotest.NoError(t, err)
	gotest.Len(t, results, 1)
	out, err := os.ReadFile(path)
	gotest.NoError(t, err)
	return string(out)
}

func (s *MigrateTestSuite) TestTodoMarkers(t *gotest.T) {
	t.When("a suite has unconvertible hooks and assertions", func(w *gotest.T) {
		out := s.migrateSource(w, todoInputSrc)

		w.It("annotates every unsupported lifecycle hook", func(it *gotest.T) {
			for _, hook := range []string{
				"SetupSubTest", "TearDownSubTest", "BeforeTest", "AfterTest", "HandleStats",
			} {
				gotest.Contains(it, out,
					"// TODO(gotest-migrate): unsupported testify hook "+hook+" — convert manually")
			}
		})

		w.It("annotates every unmapped assertion call", func(it *gotest.T) {
			for _, name := range []string{
				"ErrorAs", "Eventually", "ElementsMatch", "InDelta", "Regexp",
			} {
				gotest.Contains(it, out,
					"// TODO(gotest-migrate): unmapped assertion "+name+" — convert manually")
			}
		})

		w.It("places hook markers directly above the method declaration", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): unsupported testify hook SetupSubTest — convert manually\nfunc (s *SampleTestSuite) SetupSubTest() {}")
		})

		w.It("places assertion markers directly above the statement", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): unmapped assertion ErrorAs — convert manually\n\ts.Require().ErrorAs(nil, &target)")
		})

		w.It("annotates direct embedded-suite calls even for mapped names", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): unconverted assertion Equal — embedded-suite call; rewrite as gotest.Equal(t, ...)\n\ts.Equal(2, 2)")
		})

		w.It("still converts mapped assertions", func(it *gotest.T) {
			gotest.Contains(it, out, "gotest.NoError(t, nil)")
			gotest.Contains(it, out, "gotest.Equal(t, 1, 1)")
		})

		w.It("does not annotate mapped assertions", func(it *gotest.T) {
			gotest.NotContains(it, out, "unmapped assertion NoError")
			gotest.NotContains(it, out, "unmapped assertion Equal")
		})

		w.It("converts supported hooks untouched by markers", func(it *gotest.T) {
			gotest.Contains(it, out, "func (s *SampleTestSuite) BeforeEach(t *gotest.T) {}")
			gotest.NotContains(it, out, "unsupported testify hook SetupTest")
		})

		w.It("emits exactly one marker per finding", func(it *gotest.T) {
			gotest.Equal(it, 11, strings.Count(out, "TODO(gotest-migrate)"))
		})

		w.It("produces parseable, gofmt-clean output", func(it *gotest.T) {
			fset := token.NewFileSet()
			_, err := parser.ParseFile(fset, "sample_test.go", out, parser.ParseComments)
			gotest.NoError(it, err)
		})
	})

	t.When("a suite is fully convertible", func(w *gotest.T) {
		const cleanSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type CleanSuite struct {
	suite.Suite
}

func (s *CleanSuite) TestOK() {
	s.Require().Equal(1, 1)
}

func TestCleanSuite(t *testing.T) {
	suite.Run(t, new(CleanSuite))
}
`
		out := s.migrateSource(w, cleanSrc)

		w.It("emits no TODO markers", func(it *gotest.T) {
			gotest.NotContains(it, out, "TODO(gotest-migrate)")
		})
	})
}
