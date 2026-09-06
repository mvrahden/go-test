package gotestspec

import "strings"

// Shims exposing package internals to the external test package.

type ExportJSONRoot = jsonRoot

var (
	ExportParseFileLine       = parseFileLine
	ExportParseFirstLocation  = parseFirstLocation
	ExportPackageDir          = packageDir
	ExportStripStdlibFrames   = stripStdlibFrames
	ExportIsStdlibFile        = isStdlibFile
	ExportParseCoverageReader = parseCoverageReader
	ExportParseCoverageLine   = parseCoverageLine
	ExportRenderSummaryLine   = renderSummary
	ExportANSIColors          = ansiColors
	ExportCollectFailures     = collectFailures
	ExportStripANSI           = stripANSI
)

// stripANSI removes color escape sequences so tests compare visible text.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

var (
	ExportSplitTestPath    = splitTestPath
	ExportNoDiagnosticNote = noDiagnosticNote
)
