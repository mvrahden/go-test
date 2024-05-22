package testgen

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func Execute(args []string) error {
	var scanPath, outputFile string // TODO: parse from args
	var keepFile, skipAutogen bool
	err := parseFlags(args, &scanPath, &outputFile, &skipAutogen, &keepFile)
	if err != nil {
		return fmt.Errorf("failed parsing flags. err: %w", err)
	}

	targetDir := getTargetDir(scanPath)

	err = findAndDeleteOldGeneratedFile(targetDir)
	if os.IsNotExist(err) {
		return fmt.Errorf("failed generating code. err: no such directory %q", targetDir)
	}
	if err != nil {
		return fmt.Errorf("failed inspecting directory %q. err: %w", targetDir, err)
	}

	pkgName, srcs, err := gotestgen.Generate(targetDir)
	if err != nil {
		return fmt.Errorf("failed generating code. err: %w", err)
	}
	if len(srcs) == 0 {
		return fmt.Errorf("failed generating code: no sources to generate\n")
	}

	filePath := targetFilename(targetDir, outputFile)
	err = os.WriteFile(filePath, srcs, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed writing %q. err: %w", filePath, err)
	}
	if skipAutogen {
		return nil
	}
	// generate auto-gen
	return generateAutogen(targetDir, pkgName)
}

//go:embed static
var templates embed.FS

var (
	autogenTpl = template.Must(template.New("autogen").ParseFS(templates, "static/autogen.*"))
)

var generateAutogen = func(targetDir, pkgName string) error {
	buf := bytes.NewBuffer(nil)
	err := autogenTpl.ExecuteTemplate(buf, "autogen.go.tpl", map[string]any{
		"RepoName":    about.ShortInfo(),
		"PackageName": pkgName,
	})
	if err != nil {
		return fmt.Errorf("failed templating autogen file. err: %w", err)
	}

	filePath := targetFilename(targetDir, "gotest_autogen_test.go")
	err = os.WriteFile(filePath, buf.Bytes(), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed writing %q. err: %w", filePath, err)
	}
	return nil
}

var targetFilename = func(dir, filename string) string {
	if !strings.HasSuffix(filename, ".go") {
		filename = fmt.Sprintf("%s.go", filename)
	}
	return filepath.Join(dir, filename)
}

func parseFlags(args []string, scanPath, outputFile *string, skipAutogen, keepFile *bool) error {
	// setup flags
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(outputFile, "out", "gotest_gensuite_test", "the filename of the generated file; defaults to \"gotest_gensuite_test.go\".")
	flags.StringVar(scanPath, "dir", "", "directory of target package; defaults to CWD.")
	flags.BoolVar(skipAutogen, "skip-autogen", false, "for testing purposes: prevents deleting existing testsuite file; defaults to `false`.")
	flags.BoolVar(keepFile, "test.keepfile", false, "for testing purposes: prevents deleting existing testsuite file; defaults to `false`.")
	return flags.Parse(args)
}

func getTargetDir(scanPath string) string {
	targetDir, _ := os.Getwd() // hint: fallback value
	if len(scanPath) > 0 {
		if filepath.IsAbs(scanPath) {
			targetDir = filepath.Clean(scanPath)
		} else {
			targetDir = filepath.Join(targetDir, scanPath)
		}
	}
	return targetDir
}

var findAndDeleteOldGeneratedFile = func(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(nil)
	for _, fse := range files {
		buf.Reset()

		if fse.IsDir() {
			continue
		}
		if !strings.HasSuffix(fse.Name(), ".go") {
			continue
		}
		inspectFile := filepath.Join(dir, fse.Name())
		f, err := os.Open(inspectFile)
		if err != nil {
			return fmt.Errorf("failed opening file %q", fse.Name())
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("failed reading file info %q", fse.Name())
		}
		if fi.Size() < 74 {
			continue // hint: skip if less then size of the gen comment
		}
		_, err = io.CopyN(buf, f, 85)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed reading first %d bytes of file %q", buf.Len(), fse.Name())
		}
		if gotestast.GEN_TESTSUITE_FILE.Match(buf.Bytes()) {
			os.Remove(inspectFile)
		}
	}
	return nil
}
