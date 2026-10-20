package migrate_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigratePackageTestSuite covers migrations that span the files of a
// directory: a suite's methods, runner and references may sit in any of
// them, or in the external _test package.
type MigratePackageTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigratePackageTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 90 * time.Second}
}

// migrateDir migrates the module files make up with patterns.
func (s *MigratePackageTestSuite) migrateDir(t *gotest.T, files map[string]string, patterns ...string) pkgRun {
	return migrateModule(t, s.Mod, migrate.Options{}, files, patterns...)
}

const splitSuiteSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type UserSuite struct {
	suite.Suite
	n int
}

func TestUserSuite(t *testing.T) {
	suite.Run(t, new(UserSuite))
}
`

// splitMethodsSrc holds the suite's hooks, tests and a helper, with no
// imports of its own.
const splitMethodsSrc = `package user

func (s *UserSuite) SetupSuite() { s.n = 1 }

func (s *UserSuite) TearDownTest() {}

func (s *UserSuite) TestAdd() {
	s.Require().Equal(2, s.n+1)
	s.check()
}

func (s *UserSuite) check() {
	s.Require().Equal(1, s.n)
}
`

func (s *MigratePackageTestSuite) TestSiblingMethods(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{
		"user/suite_test.go":   splitSuiteSrc,
		"user/methods_test.go": splitMethodsSrc,
	}, "user")
	methods := run.files["user/methods_test.go"]

	t.When("a suite's hooks are declared in another file", func(w *gotest.T) {
		w.It("renames them to their gotest names, with t", func(it *gotest.T) {
			gotest.Contains(it, methods, "func (s *UserTestSuite) BeforeAll(t *gotest.T) { s.n = 1 }")
			gotest.Contains(it, methods, "func (s *UserTestSuite) AfterEach(t *gotest.T) {}")
			gotest.NotContains(it, methods, "SetupSuite")
			gotest.NotContains(it, methods, "TearDownTest")
		})
	})

	t.When("its test methods are declared there", func(w *gotest.T) {
		w.It("gives them t and rewrites their assertions", func(it *gotest.T) {
			gotest.Contains(it, methods, "func (s *UserTestSuite) TestAdd(t *gotest.T) {\n\tgotest.Equal(t, 2, s.n+1)")
		})

		w.It("adds the gotest import the file had none for", func(it *gotest.T) {
			gotest.Contains(it, methods, "import \"github.com/mvrahden/go-test/pkg/gotest\"\n")
		})
	})

	t.When("a helper there has no t", func(w *gotest.T) {
		w.It("marks its assertion", func(it *gotest.T) {
			gotest.Contains(it, methods,
				"\t// TODO(gotest-migrate): assertion Equal outside a migrated method — no t is in scope here; pass t in and rewrite as gotest.Equal(t, ...)\n\ts.Require().Equal(1, s.n)")
			gotest.Equal(it, 1, run.markers())
		})
	})

	t.It("reports the methods file as updated for the suite", func(it *gotest.T) {
		gotest.Len(it, run.results, 2)
		gotest.True(it, run.results[0].RefsOnly)
		gotest.Equal(it, "UserSuite", run.results[1].OldName)
	})
}

const runnerSuiteSrc = `package user

import "github.com/stretchr/testify/suite"

type RunSuite struct {
	suite.Suite
}

func (s *RunSuite) TestOK() {
	s.Require().True(true)
}
`

const runnerSrc = `package user

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(RunSuite))
}

func TestGuardedSuite(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("CI only")
	}
	suite.Run(t, &GuardedSuite{})
}

type GuardedSuite struct {
	suite.Suite
}

func (s *GuardedSuite) TestOK() {}
`

func (s *MigratePackageTestSuite) TestSiblingRunner(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{
		"user/suite_test.go":  runnerSuiteSrc,
		"user/runner_test.go": runnerSrc,
	}, "user")
	runner := run.files["user/runner_test.go"]

	t.When("the runner is in another file than the suite", func(w *gotest.T) {
		w.It("removes it like one in the suite's own file", func(it *gotest.T) {
			gotest.NotContains(it, runner, "TestRunSuite")
			gotest.NotContains(it, runner, "RunSuite")
		})
	})

	t.When("a runner does more than run the suite", func(w *gotest.T) {
		w.It("keeps it and marks its suite.Run", func(it *gotest.T) {
			gotest.Contains(it, runner,
				"\t// TODO(gotest-migrate): suite.Run cannot run a gotest suite — gotest generates the runner; move what TestGuardedSuite sets up into the suite (BeforeAll, SuiteGuard) and delete it\n\tsuite.Run(t, &GuardedTestSuite{})")
			gotest.Contains(it, runner, "t.Skip(\"CI only\")")
		})

		w.It("keeps the imports it still uses", func(it *gotest.T) {
			gotest.Contains(it, runner, `"github.com/stretchr/testify/suite"`)
			gotest.Contains(it, runner, `"testing"`)
		})

		w.It("counts the marker", func(it *gotest.T) {
			gotest.Equal(it, 1, run.markers())
		})
	})
}

const extInternalSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type UserSuite struct {
	suite.Suite
}

func (s *UserSuite) TestOK() {}

func TestUserSuite(t *testing.T) {
	suite.Run(t, new(UserSuite))
}
`

// extSrc declares its own UserSuite and refers to the internal one.
const extSrc = `package user_test

import (
	"testing"

	"example.com/m/user"
	"github.com/stretchr/testify/suite"
)

type UserSuite struct {
	suite.Suite
}

func (s *UserSuite) TestOK() {}

var _ = new(user.UserSuite)

func TestExt(t *testing.T) {
	suite.Run(t, new(UserSuite))
}
`

func (s *MigratePackageTestSuite) TestExternalPackage(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{
		"user/user.go":           "package user\n",
		"user/suite_test.go":     extInternalSrc,
		"user/ext_suite_test.go": extSrc,
	}, "user")
	ext := run.files["user/ext_suite_test.go"]

	t.When("the external package refers to the package's suite", func(w *gotest.T) {
		w.It("renames the qualified reference", func(it *gotest.T) {
			gotest.Contains(it, ext, "var _ = new(user.UserTestSuite)")
		})
	})

	t.When("both packages declare a suite of the same name", func(w *gotest.T) {
		w.It("migrates each in its own package", func(it *gotest.T) {
			gotest.Contains(it, ext, "type UserTestSuite struct {")
			gotest.Contains(it, ext, "func (s *UserTestSuite) TestOK(t *gotest.T) {}")
			gotest.Contains(it, run.files["user/suite_test.go"], "func (s *UserTestSuite) TestOK(t *gotest.T) {}")
			gotest.NotContains(it, ext, "TestExt")
			gotest.Len(it, run.results, 2)
		})
	})
}

// shadowSrc spells the suite's name as a field, a key and a local variable.
const shadowSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type UserSuite struct {
	suite.Suite
}

func (s *UserSuite) TestOK() {}

type cfg struct{ UserSuite int }

func TestLocal(t *testing.T) {
	var x *UserSuite
	_ = x
	UserSuite := 3
	c := cfg{UserSuite: UserSuite}
	_ = c.UserSuite
}

func TestUserSuite(t *testing.T) {
	suite.Run(t, new(UserSuite))
}
`

func (s *MigratePackageTestSuite) TestScopedRename(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{"user/suite_test.go": shadowSrc}, "user")
	out := run.files["user/suite_test.go"]

	t.It("renames what resolves to the suite type", func(it *gotest.T) {
		gotest.Contains(it, out, "type UserTestSuite struct {")
		gotest.Contains(it, out, "var x *UserTestSuite")
	})

	t.It("leaves a field, key and variable of the same spelling", func(it *gotest.T) {
		gotest.Contains(it, out, "type cfg struct{ UserSuite int }")
		gotest.Contains(it, out, "\tUserSuite := 3\n\tc := cfg{UserSuite: UserSuite}\n\t_ = c.UserSuite\n")
	})
}

const embedBaseSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
}

func TestBaseSuite(t *testing.T) {
	suite.Run(t, new(BaseSuite))
}
`

const embedChildSrc = `package user

type wrap struct{ *BaseSuite }

var _ = wrap{}
`

const embedMethodsSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func (s *BaseSuite) SetupTest() {}

func (s *BaseSuite) TestOK() {
	s.Require().True(true)
}

type OtherSuite struct {
	suite.Suite
}

func (s *OtherSuite) TestOK() {}

func TestOtherSuite(t *testing.T) {
	suite.Run(t, new(OtherSuite))
}
`

func (s *MigratePackageTestSuite) TestEmbeddingAcrossFiles(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{
		"user/base_test.go":    embedBaseSrc,
		"user/wrap_test.go":    embedChildSrc,
		"user/methods_test.go": embedMethodsSrc,
	}, "user")
	methods := run.files["user/methods_test.go"]

	t.When("a struct in another file embeds the suite", func(w *gotest.T) {
		w.It("refuses the child's file with a marker", func(it *gotest.T) {
			gotest.Contains(it, run.files["user/wrap_test.go"],
				"// TODO(gotest-migrate): wrap embeds *BaseSuite, which not every suite.Run sets to a fresh &BaseSuite{}, or other code sets; gotest creates a suite with new, which leaves that pointer nil — embed BaseSuite by value, or migrate this file by hand\ntype wrap struct{ *BaseSuite }")
		})

		w.It("refuses the suite's own file, saying why", func(it *gotest.T) {
			base := run.files["user/base_test.go"]
			gotest.Contains(it, base,
				"// TODO(gotest-migrate): BaseSuite is embedded by wrap in wrap_test.go — migrate this file by hand\ntype BaseSuite struct {")
			gotest.Contains(it, base, "suite.Run(t, new(BaseSuite))")
		})
	})

	t.When("the refused suite has methods in a file that migrates", func(w *gotest.T) {
		w.It("leaves them as they were", func(it *gotest.T) {
			gotest.Contains(it, methods, "func (s *BaseSuite) SetupTest() {}")
			gotest.Contains(it, methods, "func (s *BaseSuite) TestOK() {\n\ts.Require().True(true)\n}")
		})

		w.It("still migrates the file's other suite", func(it *gotest.T) {
			gotest.Contains(it, methods, "func (s *OtherTestSuite) TestOK(t *gotest.T) {}")
			gotest.NotContains(it, methods, "TestOtherSuite")
		})
	})

	t.It("exits on the markers", func(it *gotest.T) {
		gotest.Equal(it, 2, run.markers())
	})
}

const otherSuiteSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type OtherSuite struct {
	suite.Suite
}

func (s *OtherSuite) SetupSuite() {}

func TestOtherSuite(t *testing.T) {
	suite.Run(t, new(OtherSuite))
}
`

func (s *MigratePackageTestSuite) TestRefusalSpreads(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{
		"user/base_test.go":  embedBaseSrc,
		"user/wrap_test.go":  embedChildSrc + "\nfunc use(o *OtherSuite) {}\n",
		"user/other_test.go": otherSuiteSrc,
	}, "user")

	t.When("a refused file uses another suite", func(w *gotest.T) {
		w.It("leaves that suite as it is, saying why", func(it *gotest.T) {
			gotest.Equal(it,
				strings.Replace(otherSuiteSrc, "type OtherSuite struct {",
					"// TODO(gotest-migrate): OtherSuite is used in wrap_test.go, which is left for hand migration — migrate this file by hand\ntype OtherSuite struct {", 1),
				run.files["user/other_test.go"])
		})

		w.It("keeps the reference intact", func(it *gotest.T) {
			gotest.Contains(it, run.files["user/wrap_test.go"], "func use(o *OtherSuite) {}")
		})
	})
}

const suiteAccessorSrc = `package user

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type AccSuite struct {
	suite.Suite
}

func (s *AccSuite) TestOK() {
	r := s.Require()
	r.True(true)
}

func TestAccSuite(t *testing.T) {
	suite.Run(t, new(AccSuite))
}
`

func (s *MigratePackageTestSuite) TestSuiteMembers(t *gotest.T) {
	run := s.migrateDir(t, map[string]string{"user/suite_test.go": suiteAccessorSrc}, "user")

	t.It("marks any other use of what suite.Suite provided", func(it *gotest.T) {
		gotest.Contains(it, run.files["user/suite_test.go"],
			"\t// TODO(gotest-migrate): s.Require came from the suite.Suite embedding, which is gone — convert manually\n\tr := s.Require()")
		gotest.Equal(it, 1, run.markers())
	})
}

func (s *MigratePackageTestSuite) TestOverlappingPatterns(t *gotest.T) {
	root := t.TempDir()
	gotest.NoError(t, s.Mod.Write(root, map[string]string{"dup/a_test.go": runnerSuiteSrc}))
	var diff bytes.Buffer
	results, err := migrate.MigratePackages(
		[]string{filepath.Join(root, "dup"), root + "/..."},
		migrate.Options{DryRun: true, Out: &diff},
	)
	gotest.NoError(t, err)

	t.It("migrates a file two patterns name once", func(it *gotest.T) {
		gotest.Len(it, results, 1)
		gotest.Equal(it, 1, strings.Count(diff.String(), "+++ b/"))
	})
}
