package speccatalog_test

import (
	"github.com/mvrahden/go-test/internal/gotestscenarios"
	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const (
	cliPkg  = "github.com/mvrahden/go-test/tests/conformance/cli"
	lintPkg = "github.com/mvrahden/go-test/tests/conformance/lint"
)

// BindingTestSuite: the rules this repository adds to gotest scenarios'
// binding name what breaks them, with the place to fix.
type BindingTestSuite struct{}

func (s *BindingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// reading decodes a scenarios document and a spec --static document.
func reading(t *gotest.T, scenarios, static string) speccatalog.Reading {
	r, err := speccatalog.Decode([]byte(scenarios), []byte(static))
	gotest.NoError(t, err)
	return r
}

// versionScenario is a cli scenario whose THEN binds an It at pkg's line 12,
// labeled when and it.
func versionScenario(pkg, when, it string) gotestscenarios.Scenario {
	return gotestscenarios.Scenario{
		Capability: "cli", File: "specs/cli/spec.md", Requirement: "Version", Title: "Asking for the version", Line: 11,
		Status:     gotestscenarios.StatusBound,
		Conditions: []gotestscenarios.Clause{{Keyword: gotestscenarios.When, Text: "the version subcommand runs", Line: 12}},
		Whens:      []gotestscenarios.Place{{Package: pkg, File: "tests/conformance/cli/version_suite_test.go", Line: 11}},
		Clauses: []gotestscenarios.Clause{{Keyword: gotestscenarios.Then, Text: "exits 0", Line: 13, Status: gotestscenarios.StatusBound, Its: []gotestscenarios.Place{
			{Package: pkg, File: "tests/conformance/cli/version_suite_test.go", Line: 12, Path: []string{"TestCLIVersionTestSuite", "TestVersion", when, it}},
		}}},
	}
}

func (s *BindingTestSuite) TestNotBound(t *gotest.T) {
	doc := gotestscenarios.Document{Scenarios: []gotestscenarios.Scenario{
		versionScenario(cliPkg, "the version subcommand runs", "exits 0"),
		{Capability: "cli", File: "specs/cli/spec.md", Requirement: "Version", Title: "Reworded", Line: 20, Status: gotestscenarios.StatusUnbound},
	}}
	t.When("a document holds a bound and an unbound scenario", func(w *gotest.T) {
		w.It("names the unbound one with its status, file, line, requirement and title", func(it *gotest.T) {
			gotest.Equal(it, []string{"unbound specs/cli/spec.md:20 Version / Reworded"}, speccatalog.NotBound(&doc))
		})
		w.It("reports it as gotest scenarios does", func(it *gotest.T) {
			gotest.Regexp(it, `(?m)^unbound\s+specs/cli/spec.md:20  Version / Reworded$`, speccatalog.Report(&doc))
		})
	})
}

func (s *BindingTestSuite) TestOutsideCapability(t *gotest.T) {
	t.When("a scenario binds an It in its own capability's package", func(w *gotest.T) {
		doc := gotestscenarios.Document{Scenarios: []gotestscenarios.Scenario{versionScenario(cliPkg, "the version subcommand runs", "exits 0")}}
		w.It("is not named", func(it *gotest.T) {
			gotest.Empty(it, speccatalog.OutsideCapability(&doc))
		})
	})
	t.When("a scenario binds an It in another capability's package", func(w *gotest.T) {
		doc := gotestscenarios.Document{Scenarios: []gotestscenarios.Scenario{versionScenario(lintPkg, "the version subcommand runs", "exits 0")}}
		w.It("names the scenario, the clause, the It and the package it belongs in", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`specs/cli/spec.md:11: scenario "Asking for the version": THEN "exits 0" binds the It at tests/conformance/cli/version_suite_test.go:12, outside tests/conformance/cli`,
			}, speccatalog.OutsideCapability(&doc))
		})
	})
}

func (s *BindingTestSuite) TestInexact(t *gotest.T) {
	t.When("the labels equal the scenario's text", func(w *gotest.T) {
		doc := gotestscenarios.Document{Scenarios: []gotestscenarios.Scenario{versionScenario(cliPkg, "the version subcommand runs", "exits 0")}}
		w.It("names nothing", func(it *gotest.T) {
			gotest.Empty(it, speccatalog.Inexact(&doc))
		})
	})
	t.When("the labels differ from the text only where gotest scenarios normalizes", func(w *gotest.T) {
		doc := gotestscenarios.Document{Scenarios: []gotestscenarios.Scenario{versionScenario(cliPkg, "The `version` subcommand runs", "Exits 0.")}}
		w.It("names the WHEN and the THEN with the labels as written", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`specs/cli/spec.md:11: scenario "Asking for the version": WHEN "the version subcommand runs" is labeled "The ` + "`version`" + ` subcommand runs" at tests/conformance/cli/version_suite_test.go:12`,
				`specs/cli/spec.md:11: scenario "Asking for the version": THEN "exits 0" is labeled "Exits 0." at tests/conformance/cli/version_suite_test.go:12`,
			}, speccatalog.Inexact(&doc))
		})
	})
}

const whensTree = `{"packages":[
 {"path":"github.com/mvrahden/go-test/tests/conformance/cli","nodes":[
  {"kind":"suite","display":"CLIVersion","children":[
  {"kind":"method","display":"Version","children":[
   {"kind":"block","vocab":"when","display":"when the version subcommand runs","name":"the_version_subcommand_runs","file":"version_suite_test.go","line":11,"children":[
    {"kind":"block","vocab":"it","display":"exits 0","file":"version_suite_test.go","line":12,"children":[]}]},
   {"kind":"block","vocab":"when","display":"when a flag is unknown","name":"when_a_flag_is_unknown","file":"version_suite_test.go","line":14,"children":[
    {"kind":"block","vocab":"it","display":"exits 2","file":"version_suite_test.go","line":15,"children":[]}]},
   {"kind":"block","vocab":"when","display":"when a table has no It","name":"a_table_has_no_it","file":"version_suite_test.go","line":17,"children":[
    {"kind":"block","vocab":"each","display":"row","file":"version_suite_test.go","line":18,"children":[]}]}]}]},
  {"kind":"suite","display":"Parked","excluded":true,"children":[
  {"kind":"method","display":"Parked","children":[
   {"kind":"block","vocab":"when","display":"when the suite is parked","name":"the_suite_is_parked","file":"parked_suite_test.go","line":5,"children":[
    {"kind":"block","vocab":"it","display":"is no clause","file":"parked_suite_test.go","line":6,"children":[]}]}]}]}]},
 {"path":"github.com/mvrahden/go-test/tests/conformance/lint","nodes":[
  {"kind":"suite","display":"Lint","children":[
  {"kind":"method","display":"Rules","children":[
   {"kind":"block","vocab":"when","display":"when a rule fires","name":"a_rule_fires","file":"rules_suite_test.go","line":7,"children":[
    {"kind":"block","vocab":"it","display":"names the rule","file":"rules_suite_test.go","line":8,"children":[]}]}]}]}]}]}`

const whensDoc = `{"specs":["specs/cli/spec.md"],"scenarios":[{"capability":"cli","file":"specs/cli/spec.md","status":"bound",
 "whens":[{"package":"github.com/mvrahden/go-test/tests/conformance/cli","file":"tests/conformance/cli/version_suite_test.go","line":11}]}]}`

func (s *BindingTestSuite) TestUnboundWhens(t *gotest.T) {
	r := reading(t, whensDoc, whensTree)
	t.When("conformance Whens hold Its no scenario's WHEN binds", func(w *gotest.T) {
		w.It("names each such It with its When, its place and the spec it is missing from, skipping excluded suites and Whens a scenario binds", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`tests/conformance/cli: CLIVersion.Version: When "when a flag is unknown" → It "exits 2" (version_suite_test.go:15) is in no scenario of specs/cli/spec.md`,
				`tests/conformance/lint: Lint.Rules: When "a rule fires" → It "names the rule" (rules_suite_test.go:8) is in no scenario of specs/lint/spec.md, which does not exist`,
			}, speccatalog.UnboundWhens(&r))
		})
	})
}

const tableTree = `{"packages":[{"path":"github.com/mvrahden/go-test/tests/conformance/cli","nodes":[
 {"kind":"suite","display":"CLIFlags","children":[
 {"kind":"method","display":"Spellings","children":[
  {"kind":"block","vocab":"when","display":"when a flag is spelled two ways","name":"a_flag_is_spelled_two_ways","children":[
   {"kind":"block","vocab":"each","display":"--x","children":[]},
   {"kind":"block","vocab":"each","display":"--x=true","children":[]}]},
  {"kind":"block","vocab":"when","display":"when each row holds an It","name":"each_row_holds_an_it","children":[
   {"kind":"block","vocab":"each","display":"one","children":[
    {"kind":"block","vocab":"it","display":"passes","children":[]}]}]},
  {"kind":"block","vocab":"when","display":"when the table is stated","name":"the_table_is_stated","children":[
   {"kind":"block","vocab":"it","display":"reads every spelling","children":[
    {"kind":"block","vocab":"each","display":"--x","children":[]}]}]},
  {"kind":"block","vocab":"when","display":"when the table is parked","name":"the_table_is_parked","excluded":true,"children":[
   {"kind":"block","vocab":"each","display":"--x","children":[]}]}]}]}]}]}`

func (s *BindingTestSuite) TestUnstatedTables(t *gotest.T) {
	r := reading(t, "{}", tableTree)
	t.When("a When holds Each rows and no It of its own", func(w *gotest.T) {
		w.It("is named with its package, suite and method", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`tests/conformance/cli: CLIFlags.Spellings: When "a flag is spelled two ways"`,
				`tests/conformance/cli: CLIFlags.Spellings: When "each row holds an It"`,
			}, speccatalog.UnstatedTables(&r))
		})
	})
}

const nestedTree = `{"packages":[{"path":"github.com/mvrahden/go-test/tests/conformance/cli","nodes":[
 {"kind":"suite","display":"CLIFlags","children":[
 {"kind":"method","display":"Nesting","children":[
  {"kind":"block","vocab":"it","display":"stands under no When","children":[]},
  {"kind":"block","vocab":"when","display":"when a flag is given","name":"a_flag_is_given","children":[
   {"kind":"block","vocab":"it","display":"is a clause","children":[
    {"kind":"block","vocab":"it","display":"stands under an It","children":[]}]},
   {"kind":"block","vocab":"each","display":"--x","children":[
    {"kind":"block","vocab":"it","display":"stands under a row","children":[]}]}]},
  {"kind":"block","vocab":"when","display":"when a flag is parked","name":"a_flag_is_parked","children":[
   {"kind":"block","vocab":"each","display":"--y","excluded":true,"children":[
    {"kind":"block","vocab":"it","display":"is skipped","children":[]}]}]}]}]}]}]}`

func (s *BindingTestSuite) TestMisplacedIts(t *gotest.T) {
	r := reading(t, "{}", nestedTree)
	t.When("a tree holds Its under no When, under an It and under Each rows", func(w *gotest.T) {
		w.It("names every It not directly under a When with its package, suite, method and placement", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`tests/conformance/cli: CLIFlags.Nesting: It "stands under no When" has no When above it`,
				`tests/conformance/cli: CLIFlags.Nesting: It "stands under an It" is under It "is a clause"`,
				`tests/conformance/cli: CLIFlags.Nesting: It "stands under a row" is under Each row "--x"`,
			}, speccatalog.MisplacedIts(&r))
		})
	})
}
