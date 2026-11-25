package gotestscenarios

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultDirs are where a module's specs are read from when nothing names
// other places: gotest's own layout and OpenSpec's.
var DefaultDirs = []string{"specs", "openspec/specs"}

// SpecFile is the name every capability's spec has.
const SpecFile = "spec.md"

// Load reads the spec files under each path, a spec file or a directory,
// relative to base; with no path, the DefaultDirs under root that exist.
// A capability is a spec file's directory below the one named. Files are
// named relative to root.
func Load(root, base string, paths []string) ([]Spec, error) {
	explicit := len(paths) > 0
	if !explicit {
		base = root
		paths = DefaultDirs
	}
	var specs []Spec
	seen := map[string]bool{}
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(base, p)
		}
		info, err := os.Stat(abs)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !explicit:
			continue
		case err != nil:
			return nil, err
		case !info.IsDir():
			spec, err := loadFile(root, abs, filepath.Base(filepath.Dir(abs)))
			if err != nil {
				return nil, err
			}
			if !seen[spec.File] {
				seen[spec.File] = true
				specs = append(specs, spec)
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != SpecFile {
				return err
			}
			capability, err := filepath.Rel(abs, filepath.Dir(path))
			if err != nil {
				return err
			}
			if capability == "." {
				capability = filepath.Base(abs)
			}
			spec, err := loadFile(root, path, filepath.ToSlash(capability))
			if err != nil {
				return err
			}
			if !seen[spec.File] {
				seen[spec.File] = true
				specs = append(specs, spec)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("no %s under %s", SpecFile, strings.Join(paths, ", "))
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].File < specs[j].File })
	return specs, nil
}

func loadFile(root, path, capability string) (Spec, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a spec file the user's tree holds
	if err != nil {
		return Spec{}, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	return Parse(capability, filepath.ToSlash(rel), data), nil
}
