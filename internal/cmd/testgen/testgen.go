package testgen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

type GenerateResult struct {
	AbsPath string
	Package string
	PTest   []byte
	PXTest  []byte
}

func GenerateSuites(path string) (GenerateResult, error) {
	err := findAndDeleteOldGeneratedFile(path)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("failed inspecting directory %q: %w", path, err)
	}

	pkgDir, pkgPath, ptestSrcs, pxtestSrcs, err := gotestgen.Generate(path)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("failed generating code: %w", err)
	}

	return GenerateResult{AbsPath: pkgDir, Package: pkgPath, PTest: ptestSrcs, PXTest: pxtestSrcs}, nil
}

var findAndDeleteOldGeneratedFile = func(dir string) error {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
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
