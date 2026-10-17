// Package skills embeds the agent skills that `gotest skill install` writes
// into a project, so the installed copy always matches the CLI's version.
// It exists for the gotest CLI and is not a stable API.
package skills

import "embed"

// FS holds each skill's SKILL.md and reference files.
//
//go:embed writing-gotest-tests/SKILL.md writing-gotest-tests/reference
var FS embed.FS

// Name is the directory and frontmatter name of the embedded skill.
const Name = "writing-gotest-tests"
