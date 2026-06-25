package gotestspec

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Annotation struct {
	File    string
	Line    int
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

		if dir != "" {
			file = dir + "/" + file
		}

		annotations = append(annotations, Annotation{
			File:    file,
			Line:    line,
			Title:   title,
			Message: msg,
		})
	}
	return annotations
}

func WriteGitHubAnnotations(w io.Writer, annotations []Annotation) {
	for _, a := range annotations {
		msg := a.Message
		if len(msg) > 1024 {
			msg = msg[:1021] + "..."
		}
		msg = strings.ReplaceAll(msg, "\n", "%0A")
		msg = strings.ReplaceAll(msg, "\r", "")

		if a.Line > 0 {
			fmt.Fprintf(w, "::error file=%s,line=%d,title=%s::%s\n",
				a.File, a.Line, a.Title, msg)
		} else {
			fmt.Fprintf(w, "::error file=%s,title=%s::%s\n",
				a.File, a.Title, msg)
		}
	}
}

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
	for _, l := range lines {
		f, ln, msg := parseFileLine(l)
		if f != "" {
			var allMsgs []string
			if msg != "" {
				allMsgs = append(allMsgs, msg)
			}
			for _, rest := range lines {
				if rest == l {
					continue
				}
				rf, _, rm := parseFileLine(rest)
				if rf == "" && rm == "" {
					allMsgs = append(allMsgs, rest)
				} else if rm != "" {
					allMsgs = append(allMsgs, rm)
				}
			}
			return f, ln, strings.Join(allMsgs, "\n")
		}
	}

	if len(lines) > 0 {
		return "", 0, strings.Join(lines, "\n")
	}
	return "", 0, ""
}

func parseFileLine(s string) (file string, line int, message string) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ".go:") {
		return "", 0, ""
	}

	idx := strings.Index(s, ".go:")
	fileEnd := idx + 3
	file = s[:fileEnd]

	rest := s[fileEnd+1:]
	lineStr, after, ok := strings.Cut(rest, ":")
	if !ok {
		return "", 0, ""
	}
	ln, err := strconv.Atoi(lineStr)
	if err != nil {
		return "", 0, ""
	}

	message = strings.TrimSpace(after)
	return file, ln, message
}
