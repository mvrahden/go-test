package speccatalog_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const sampleSpec = "# CLI Specification\n" +
	"\n" +
	"## Purpose\n" +
	"How the gotest command line answers the invocations it is given.\n" +
	"\n" +
	"## Requirements\n" +
	"\n" +
	"### Requirement: Version\n" +
	"The version subcommand SHALL exit 0 and print the version.\n" +
	"\n" +
	"#### Scenario: Asking for the version\n" +
	"- **WHEN** the version subcommand runs\n" +
	"- **THEN** exits 0 and prints the version on stdout\n" +
	"- **AND** prints nothing on stderr\n" +
	"\n" +
	"```markdown\n" +
	"### Requirement: Inside a fence\n" +
	"#### Scenario: Not a scenario\n" +
	"- **WHEN** fenced\n" +
	"```\n" +
	"\n" +
	"### Requirement: Streams\n" +
	"A JSON stream MUST hold JSON lines only.\n" +
	"\n" +
	"#### Scenario: The test stream\n" +
	"- **WHEN** the go test -json stream is asked for\n" +
	"- **THEN** holds JSON lines only\n" +
	"\n" +
	"#### Another heading\n" +
	"- **WHEN** a JSON document is asked for\n" +
	"- **THEN** prints one document\n"

// SpecTestSuite: a spec file is read in OpenSpec's shape, and a file that
// breaks the shape is named with what to fix.
type SpecTestSuite struct{}

func (s *SpecTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SpecTestSuite) TestParse(t *gotest.T) {
	spec := speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(sampleSpec))
	t.When("a spec holds several requirements and a fenced example", func(w *gotest.T) {
		w.It("reads the title, the purpose and every requirement outside the fence", func(it *gotest.T) {
			gotest.Equal(it, "CLI Specification", spec.Title)
			gotest.Equal(it, "How the gotest command line answers the invocations it is given.", spec.Purpose)
			gotest.Len(it, spec.Requirements, 2)
			gotest.Equal(it, "Version", spec.Requirements[0].Name)
			gotest.Equal(it, "The version subcommand SHALL exit 0 and print the version.", spec.Requirements[0].Text)
			gotest.Equal(it, "Streams", spec.Requirements[1].Name)
		})
		w.It("reads each scenario's WHEN and its THEN and AND bullets in order", func(it *gotest.T) {
			gotest.Equal(it, speccatalog.Scenario{
				Title: "Asking for the version",
				Line:  11,
				Whens: []string{"the version subcommand runs"},
				Thens: []string{"exits 0 and prints the version on stdout", "prints nothing on stderr"},
			}, spec.Requirements[0].Scenarios[0])
		})
		w.It("takes every level-four heading under a requirement as a scenario", func(it *gotest.T) {
			gotest.Len(it, spec.Requirements[1].Scenarios, 2)
			gotest.Equal(it, "Another heading", spec.Requirements[1].Scenarios[1].Title)
		})
		w.It("reports no shape problem", func(it *gotest.T) {
			gotest.Empty(it, spec.Problems())
		})
	})
	t.When("the spec has CRLF line endings", func(w *gotest.T) {
		crlf := speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(strings.ReplaceAll(sampleSpec, "\n", "\r\n")))
		w.It("reads the same spec", func(it *gotest.T) {
			gotest.Equal(it, spec, crlf)
		})
	})
}

const shapeSpec = "# Broken Specification\n" +
	"\n" +
	"## Purpose\n" +
	"Too short.\n" +
	"\n" +
	"## Requirements\n" +
	"\n" +
	"### Requirement: No keyword\n" +
	"The command should exit 0.\n" +
	"\n" +
	"#### Scenario: No WHEN\n" +
	"- **THEN** exits 0\n" +
	"\n" +
	"#### Scenario: No THEN\n" +
	"- **WHEN** the command runs\n" +
	"\n" +
	"### Requirement: Empty\n" +
	"It MUST do something.\n" +
	"\n" +
	"### Requirement: No keyword\n" +
	"It SHALL be named once.\n" +
	"\n" +
	"#### Scenario: Wrapped bullet\n" +
	"- **WHEN** a long condition\n" +
	"  wraps onto a second line\n" +
	"- **THEN** is one bullet\n"

const misplacedSpec = "# CLI Specification\n" +
	"\n" +
	"## Purpose\n" +
	"#### Scenario: Before any requirement\n" +
	"- **WHEN** nothing holds it\n" +
	"How the gotest command line answers the invocations it is given.\n" +
	"### Requirement: In the purpose\n" +
	"It SHALL not count.\n" +
	"\n" +
	"## Requirements\n" +
	"\n" +
	"### Requirement: Version\n" +
	"The version subcommand SHALL exit 0.\n" +
	"\n" +
	"#### Scenario: Asking for the version\n" +
	"- **WHEN** the version subcommand runs\n" +
	"- **THEN** exits 0\n" +
	"\n" +
	"\n" +
	"### Requirement Typo without colon\n" +
	"It SHALL be bound.\n" +
	"\n" +
	"#### Scenario: Under the typo\n" +
	"- **WHEN** the typo holds a scenario\n" +
	"- **THEN** binds to nothing\n" +
	"\n" +
	"### Requirement: Bullet first\n" +
	"- **WHEN** a bullet comes before any scenario\n" +
	"It SHALL have a scenario.\n" +
	"\n" +
	"#### Scenario: The scenario\n" +
	"- **WHEN** the scenario follows\n" +
	"- **THEN** it binds\n"

func (s *SpecTestSuite) TestShape(t *gotest.T) {
	broken := speccatalog.ParseSpec("broken", "specs/broken/spec.md", []byte(shapeSpec))
	problems := broken.Problems()
	t.When("a spec breaks the shape", func(w *gotest.T) {
		w.It("names each problem with the file and the line to fix", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"specs/broken/spec.md: Purpose has 10 characters, needs at least 50",
				`specs/broken/spec.md:8: requirement "No keyword": text contains neither SHALL nor MUST`,
				`specs/broken/spec.md:11: scenario "No WHEN": has no WHEN bullet`,
				`specs/broken/spec.md:14: scenario "No THEN": has no THEN bullet`,
				`specs/broken/spec.md:17: requirement "Empty": has no scenario`,
				`specs/broken/spec.md:20: requirement "No keyword": name is already used at line 8`,
				`specs/broken/spec.md:25: scenario "Wrapped bullet": line is not a WHEN, THEN or AND bullet: wraps onto a second line`,
			}, problems)
		})
	})
	t.When("a requirement heading is malformed or outside the Requirements section", func(w *gotest.T) {
		spec := speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(misplacedSpec))
		w.It("names each heading and bullet that belongs to no requirement", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"specs/cli/spec.md:4: scenario heading belongs to no requirement: #### Scenario: Before any requirement",
				"specs/cli/spec.md:5: bullet belongs to no requirement: - **WHEN** nothing holds it",
				"specs/cli/spec.md:7: requirement heading is outside ## Requirements: ### Requirement: In the purpose",
				`specs/cli/spec.md:20: heading under ## Requirements does not start with "Requirement:": ### Requirement Typo without colon`,
				"specs/cli/spec.md:23: scenario heading belongs to no requirement: #### Scenario: Under the typo",
				"specs/cli/spec.md:24: bullet belongs to no requirement: - **WHEN** the typo holds a scenario",
				"specs/cli/spec.md:25: bullet belongs to no requirement: - **THEN** binds to nothing",
				"specs/cli/spec.md:28: bullet belongs to no scenario: - **WHEN** a bullet comes before any scenario",
			}, spec.Problems())
		})
	})
	t.When("a requirement's text is longer than 500 characters", func(w *gotest.T) {
		long := strings.Replace(sampleSpec, "The version subcommand SHALL exit 0 and print the version.", "It SHALL "+strings.Repeat("x", 500), 1)
		spec := speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(long))
		w.It("is named", func(it *gotest.T) {
			gotest.Equal(it, []string{`specs/cli/spec.md:8: requirement "Version": text has 509 characters, at most 500`}, spec.Problems())
		})
	})
	t.When("a spec has no requirement", func(w *gotest.T) {
		bare := "# Bare Specification\n\n## Purpose\nA purpose long enough to pass the fifty character floor.\n\n## Requirements\n"
		spec := speccatalog.ParseSpec("bare", "specs/bare/spec.md", []byte(bare))
		w.It("is named", func(it *gotest.T) {
			gotest.Equal(it, []string{"specs/bare/spec.md: has no requirement"}, spec.Problems())
		})
	})
}

const longFenceSpec = "# CLI Specification\n" +
	"\n" +
	"## Purpose\n" +
	"How the gotest command line answers the invocations it is given.\n" +
	"\n" +
	"## Requirements\n" +
	"\n" +
	"### Requirement: Version\n" +
	"The version subcommand SHALL exit 0.\n" +
	"\n" +
	"#### Scenario: Asking for the version\n" +
	"- **WHEN** the version subcommand runs\n" +
	"- **THEN** exits 0\n" +
	"\n" +
	"````markdown\n" +
	"```\n" +
	"- **WHEN** inside\n" +
	"### Requirement: Inside the fence\n" +
	"````\n" +
	"\n" +
	"### Requirement: After the fence\n" +
	"It SHALL be read.\n" +
	"\n" +
	"#### Scenario: Read\n" +
	"- **WHEN** the fence has closed\n" +
	"- **THEN** is read\n"

func (s *SpecTestSuite) TestFences(t *gotest.T) {
	spec := speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(longFenceSpec))
	t.When("a four-backtick fence holds a three-backtick line", func(w *gotest.T) {
		w.It("stays open until a closing line as long as its opener", func(it *gotest.T) {
			gotest.Len(it, spec.Requirements, 2)
			gotest.Equal(it, []string{"the version subcommand runs"}, spec.Requirements[0].Scenarios[0].Whens)
			gotest.Equal(it, "After the fence", spec.Requirements[1].Name)
			gotest.Empty(it, spec.Problems())
		})
	})
}
