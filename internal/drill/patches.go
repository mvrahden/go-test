package drill

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// StalePatches returns the mutants of root, sorted, whose patch no longer
// applies exactly. It changes no file.
func StalePatches(root string) (stale []string, checked int, err error) {
	dir := filepath.Join(root, "tests", "drill", "mutants")
	patches, _ := filepath.Glob(filepath.Join(dir, "*.patch"))
	if len(patches) == 0 {
		return nil, 0, fmt.Errorf("no mutants under %s", dir)
	}
	sort.Strings(patches)
	for _, p := range patches {
		// The worker's own flags, plus --dry-run.
		cmd := exec.Command("patch", "-p1", "-s", "--fuzz=0", "--forward", "--dry-run", "-r", "-", "-i", p)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				return nil, 0, err
			}
			stale = append(stale, strings.TrimSuffix(filepath.Base(p), ".patch"))
		}
		checked++
	}
	return stale, checked, nil
}

// checkPatches prints one line per stale patch and the verdict.
func checkPatches(root string) (int, error) {
	if root == "" {
		return 0, errors.New("no --root; run the drill through tests/drill/drill.sh")
	}
	stale, checked, err := StalePatches(root)
	if err != nil {
		return 0, err
	}
	for _, name := range stale {
		fmt.Printf("DRILL  %s: patch does not apply, refresh it\n", name)
	}
	if len(stale) > 0 {
		fmt.Printf("DRILL  %d of %d patches do not apply\n", len(stale), checked)
		return 1, nil
	}
	fmt.Printf("drill  %d patches apply\n", checked)
	return 0, nil
}
