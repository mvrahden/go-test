package speccatalog_test

import (
	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const bindingCatalog = `{"packages":[
 {"path":"github.com/mvrahden/go-test/tests/conformance/cli","nodes":[{"kind":"suite","display":"CLIVersion","children":[
  {"kind":"method","display":"Version","children":[
   {"kind":"block","vocab":"when","display":"when the version subcommand runs","name":"the_version_subcommand_runs","children":[
    {"kind":"block","vocab":"it","display":"exits 0 and prints the version on stdout","children":[]},
    {"kind":"block","vocab":"it","display":"prints nothing on stderr","children":[]}]},
   {"kind":"block","vocab":"when","display":"when a flag is unknown","name":"when_a_flag_is_unknown","children":[
    {"kind":"block","vocab":"it","display":"exits 2","children":[]}]}]}]}]},
 {"path":"github.com/mvrahden/go-test/tests/conformance/lint","nodes":[{"kind":"suite","display":"Lint","children":[
  {"kind":"method","display":"Rules","children":[
   {"kind":"block","vocab":"when","display":"when a rule fires","name":"a_rule_fires","children":[
    {"kind":"block","vocab":"it","display":"names the rule","children":[]}]}]}]}]}]}`

const bindingSpec = "# CLI Specification\n\n## Purpose\nHow the gotest command line answers the invocations it is given.\n\n## Requirements\n\n" +
	"### Requirement: Version\nThe version subcommand SHALL print the version.\n\n" +
	"#### Scenario: Asking for the version\n" +
	"- **WHEN** the version subcommand runs\n" +
	"- **THEN** exits 0 and prints the version on stdout\n" +
	"- **AND** prints nothing on stderr\n\n" +
	"#### Scenario: Unknown flag\n" +
	"- **WHEN** when a flag is unknown\n" +
	"- **THEN** exits 2\n"

// BindingTestSuite: every scenario of a spec is a conformance clause and
// every clause is a scenario, per capability, and each side names what the
// other lacks.
type BindingTestSuite struct{}

func (s *BindingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func clauses(t *gotest.T) []speccatalog.Clause {
	cs, err := speccatalog.ClausesOf([]byte(bindingCatalog))
	gotest.NoError(t, err)
	return cs
}

func cliSpec(text string) speccatalog.Spec {
	return speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(text))
}

func lintSpec() speccatalog.Spec {
	return speccatalog.ParseSpec("lint", "specs/lint/spec.md", []byte("# Lint Specification\n\n## Purpose\nWhat the linter reports and how it names each finding.\n\n## Requirements\n\n"+
		"### Requirement: Findings\nA finding SHALL name its rule.\n\n#### Scenario: A rule fires\n- **WHEN** a rule fires\n- **THEN** names the rule\n"))
}

func (s *BindingTestSuite) TestClausesOf(t *gotest.T) {
	t.When("a catalog is read for clauses", func(w *gotest.T) {
		w.It("yields each When and It label as written, with the conformance package as capability", func(it *gotest.T) {
			gotest.Equal(it, []speccatalog.Clause{
				{Capability: "cli", When: "the version subcommand runs", It: "exits 0 and prints the version on stdout"},
				{Capability: "cli", When: "the version subcommand runs", It: "prints nothing on stderr"},
				{Capability: "cli", When: "when a flag is unknown", It: "exits 2"},
				{Capability: "lint", When: "a rule fires", It: "names the rule"},
			}, clauses(it))
		})
	})
}

const excludedCatalog = `{"packages":[{"path":"github.com/mvrahden/go-test/tests/conformance/cli","nodes":[
 {"kind":"suite","display":"Parked","excluded":true,"children":[
 {"kind":"method","display":"Parked","children":[
  {"kind":"block","vocab":"when","display":"when the suite is parked","name":"the_suite_is_parked","children":[
   {"kind":"block","vocab":"it","display":"is not a sentence anyone may quote","children":[]}]}]}]},
 {"kind":"suite","display":"Live","children":[
 {"kind":"method","display":"Live","children":[
  {"kind":"block","vocab":"when","display":"when the suite runs","name":"the_suite_runs","children":[
   {"kind":"block","vocab":"it","display":"is a sentence","children":[]}]}]}]}]}]}`

func (s *BindingTestSuite) TestExcludedSuitesYieldNoClauses(t *gotest.T) {
	cs, err := speccatalog.ClausesOf([]byte(excludedCatalog))
	gotest.NoError(t, err)
	t.When("a catalog holds an excluded suite beside a live one", func(w *gotest.T) {
		w.It("yields the live suite's clauses alone", func(it *gotest.T) {
			gotest.Equal(it, []speccatalog.Clause{{Capability: "cli", When: "the suite runs", It: "is a sentence"}}, cs)
		})
	})
}

func (s *BindingTestSuite) TestBind(t *gotest.T) {
	t.When("every scenario equals a clause and every clause a scenario", func(w *gotest.T) {
		w.It("reports nothing", func(it *gotest.T) {
			gotest.Equal(it, speccatalog.Binding{}, speccatalog.Bind([]speccatalog.Spec{cliSpec(bindingSpec), lintSpec()}, clauses(it)))
		})
	})
	t.When("a scenario's WHEN or THEN matches no clause", func(w *gotest.T) {
		spec := cliSpec(bindingSpec + "\n#### Scenario: Reworded\n- **WHEN** the version command runs\n- **THEN** exits 0\n\n#### Scenario: Extra outcome\n- **WHEN** the version subcommand runs\n- **THEN** prints a newline\n")
		b := speccatalog.Bind([]speccatalog.Spec{spec, lintSpec()}, clauses(w))
		w.It("names the scenario, its line and the unbound text", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`specs/cli/spec.md:20: scenario "Reworded": WHEN "the version command runs" is no When label in tests/conformance/cli`,
				`specs/cli/spec.md:24: scenario "Extra outcome": WHEN "the version subcommand runs" is already the WHEN of the scenario at line 11`,
			}, b.Unverified)
			gotest.Empty(it, b.Unspecified)
		})
	})
	t.When("a THEN is no It of its WHEN", func(w *gotest.T) {
		spec := cliSpec(bindingSpec + "- **AND** prints a newline\n")
		w.It("names the THEN", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`specs/cli/spec.md:16: scenario "Unknown flag": THEN "prints a newline" is no It under When "when a flag is unknown" in tests/conformance/cli`,
			}, speccatalog.Bind([]speccatalog.Spec{spec, lintSpec()}, clauses(it)).Unverified)
		})
	})
	t.When("a clause is in no scenario", func(w *gotest.T) {
		spec := cliSpec(bindingSpec[:len(bindingSpec)-len("- **AND** prints nothing on stderr\n\n#### Scenario: Unknown flag\n- **WHEN** when a flag is unknown\n- **THEN** exits 2\n")])
		b := speccatalog.Bind([]speccatalog.Spec{spec, lintSpec()}, clauses(w))
		w.It("names the capability, the clause and the spec file it is missing from", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`tests/conformance/cli: When "the version subcommand runs" → It "prints nothing on stderr" is in no scenario of specs/cli/spec.md`,
				`tests/conformance/cli: When "when a flag is unknown" → It "exits 2" is in no scenario of specs/cli/spec.md`,
			}, b.Unspecified)
			gotest.Empty(it, b.Unverified)
		})
	})
	t.When("a capability with clauses has no spec file", func(w *gotest.T) {
		b := speccatalog.Bind([]speccatalog.Spec{cliSpec(bindingSpec)}, clauses(w))
		w.It("names each of its clauses and the missing file", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`tests/conformance/lint: When "a rule fires" → It "names the rule": specs/lint/spec.md does not exist`,
			}, b.Unspecified)
		})
	})
	t.When("a spec file has no clauses", func(w *gotest.T) {
		other := speccatalog.ParseSpec("other", "specs/other/spec.md", []byte(bindingSpec))
		b := speccatalog.Bind([]speccatalog.Spec{cliSpec(bindingSpec), lintSpec(), other}, clauses(w))
		w.It("is named once", func(it *gotest.T) {
			gotest.Equal(it, []string{"specs/other/spec.md: tests/conformance/other declares no clauses"}, b.Unverified)
		})
	})
}
