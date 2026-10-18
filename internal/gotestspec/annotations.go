package gotestspec

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Annotation struct {
	File    string
	Line    int
	Col     int
	Title   string
	Message string
}

func CollectAnnotations(packages []*Package, modulePath string) []Annotation {
	failures := collectFailures(packages)
	var annotations []Annotation

	for _, f := range failures {
		dir := packageDir(f.Package, modulePath)
		title := strings.Join(f.Display, " / ")

		file, line, msg := parseFirstLocation(filterOutput(f.Output))
		if file == "" {
			continue
		}

		file = resolveAnnotationPath(file, dir)

		annotations = append(annotations, Annotation{
			File:    file,
			Line:    line,
			Title:   title,
			Message: msg,
		})
	}

	for _, pkg := range packages {
		if pkg.Status != StatusFail || len(pkg.Output) == 0 {
			continue
		}

		dir := packageDir(pkg.Path, modulePath)
		file, line, msg := parseFirstLocation(filterOutput(pkg.Output))
		if file == "" {
			continue
		}

		file = resolveAnnotationPath(file, dir)

		annotations = append(annotations, Annotation{
			File:    file,
			Line:    line,
			Title:   diagnosticTitle(pkg.Output),
			Message: msg,
		})
	}

	return annotations
}

func resolveAnnotationPath(file, pkgDir string) string {
	if len(file) == 0 {
		return file
	}
	if pkgDir != "" {
		return pkgDir + "/" + file
	}
	return file
}

func diagnosticTitle(output []string) string {
	for _, line := range output {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "WARNING: DATA RACE") {
			return "data race detected"
		}
		if strings.HasPrefix(trimmed, "panic:") {
			return trimmed
		}
		if strings.HasPrefix(trimmed, "fatal error:") {
			return trimmed
		}
	}
	return "package-level failure"
}

// annotationCap bounds an annotation's message, in bytes.
const annotationCap = 1024

// WriteGitHubAnnotations writes each annotation as an ::error workflow
// command, its message cut to annotationCap on a rune boundary.
func WriteGitHubAnnotations(w io.Writer, annotations []Annotation) {
	for _, a := range annotations {
		msg := escapeData(truncateRunes(a.Message, annotationCap))
		props := "file=" + escapeProperty(a.File)
		if a.Line > 0 {
			props += ",line=" + strconv.Itoa(a.Line)
			if a.Col > 0 {
				props += ",col=" + strconv.Itoa(a.Col)
			}
		}
		props += ",title=" + escapeProperty(a.Title)
		fmt.Fprintf(w, "::error %s::%s\n", props, msg)
	}
}

// truncateRunes cuts s to at most limit bytes, "..." included, never
// inside a multibyte character.
func truncateRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit - len("...")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// escapeData escapes a workflow command's message.
func escapeData(s string) string {
	return dataEscaper.Replace(s)
}

// escapeProperty escapes a workflow command's property value.
func escapeProperty(s string) string {
	return propertyEscaper.Replace(s)
}

var (
	dataEscaper     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	propertyEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

func ReadModulePath(dir string) string {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if mod, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(mod)
		}
	}
	_ = scanner.Err()
	return ""
}

func packageDir(pkgPath, modulePath string) string {
	if modulePath == "" {
		return pkgPath
	}
	rel, ok := strings.CutPrefix(pkgPath, modulePath)
	if !ok {
		return pkgPath
	}
	return strings.TrimPrefix(rel, "/")
}

func parseFirstLocation(lines []string) (file string, line int, message string) {
	var firstFile string
	var firstLine int
	var firstIdx int
	firstFound := false

	for i, l := range lines {
		f, ln, _ := parseFileLine(l)
		if f == "" {
			continue
		}
		if !firstFound {
			firstFile, firstLine, firstIdx = f, ln, i
			firstFound = true
		}
		if isStdlibFile(f) || f == "testing.go" {
			continue
		}
		return f, ln, gatherMessage(lines, i)
	}

	if firstFound {
		return firstFile, firstLine, gatherMessage(lines, firstIdx)
	}

	if len(lines) > 0 {
		return "", 0, strings.Join(lines, "\n")
	}
	return "", 0, ""
}

func gatherMessage(lines []string, fromIdx int) string {
	_, _, msg := parseFileLine(lines[fromIdx])
	var allMsgs []string
	if msg != "" {
		allMsgs = append(allMsgs, msg)
	}
	for _, rest := range lines[fromIdx+1:] {
		rf, _, rm := parseFileLine(rest)
		switch {
		case rf == "" && rm == "":
			allMsgs = append(allMsgs, rest)
		case rm != "":
			allMsgs = append(allMsgs, rm)
		case rf != "":
			allMsgs = append(allMsgs, rest)
		}
	}
	return stripStdlibFrames(strings.Join(allMsgs, "\n"))
}

func stripStdlibFrames(msg string) string {
	lines := strings.Split(msg, "\n")
	out := lines[:0]
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) {
			f, _, _ := parseFileLine(lines[i+1])
			if f != "" && (isStdlibFile(f) || isTestMainFile(f)) {
				i++
				continue
			}
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}

func isTestMainFile(file string) bool {
	return file == "_testmain.go" || strings.HasSuffix(file, "/_testmain.go")
}

func parseFileLine(s string) (file string, line int, message string) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ".go:") {
		return "", 0, ""
	}

	idx := strings.Index(s, ".go:")
	fileEnd := idx + 3
	rawFile := s[:fileEnd]

	rest := s[fileEnd+1:]

	// Standard format: file.go:LINE: MESSAGE
	lineStr, after, hasColon := strings.Cut(rest, ":")
	if hasColon {
		ln, err := strconv.Atoi(strings.TrimSpace(lineStr))
		if err == nil {
			return resolveBasename(rawFile), ln, strings.TrimSpace(after)
		}
	}

	// Stack trace format: file.go:LINE +0xHEX (or just file.go:LINE)
	digits := strings.TrimSpace(rest)
	end := 0
	for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
		end++
	}
	if end == 0 {
		return "", 0, ""
	}
	ln, err := strconv.Atoi(digits[:end])
	if err != nil {
		return "", 0, ""
	}

	resolved := resolveBasename(rawFile)
	if isStdlibFile(rawFile) {
		return rawFile, ln, ""
	}
	return resolved, ln, ""
}

func resolveBasename(file string) string {
	if len(file) > 0 && (file[0] == '/' || (len(file) > 1 && file[1] == ':')) {
		return filepath.Base(file)
	}
	return file
}

func isStdlibFile(file string) bool {
	idx := strings.LastIndex(file, "/src/")
	if idx < 0 {
		return false
	}
	after := file[idx+5:]
	seg, _, _ := strings.Cut(after, "/")
	return seg != "" && !strings.Contains(seg, ".")
}
