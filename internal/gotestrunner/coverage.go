package gotestrunner

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// MergeCoverProfiles merges multiple Go coverage profile files into a single
// output file. For overlapping statements, the maximum count is kept.
func MergeCoverProfiles(profiles []string, output string) error {
	var mode string
	type stmtKey struct{ file, block string }
	merged := map[stmtKey]int{}

	for _, path := range profiles {
		f, err := os.Open(path)
		if err != nil {
			continue // skip missing profiles (suites with no coverage)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if after, ok := strings.CutPrefix(line, "mode:"); ok {
				if mode == "" {
					mode = strings.TrimSpace(after)
				}
				continue
			}
			parts := strings.Fields(line)
			if len(parts) != 3 {
				continue
			}
			key := stmtKey{file: parts[0], block: parts[1]}
			count, err := strconv.Atoi(parts[2])
			if err != nil {
				continue
			}
			if existing, ok := merged[key]; !ok || count > existing {
				merged[key] = count
			}
		}
		f.Close()
	}

	if mode == "" {
		mode = "set"
	}

	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create merged profile: %w", err)
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	fmt.Fprintf(w, "mode: %s\n", mode)
	keys := make([]stmtKey, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].file != keys[j].file {
			return keys[i].file < keys[j].file
		}
		return keys[i].block < keys[j].block
	})
	for _, key := range keys {
		fmt.Fprintf(w, "%s %s %d\n", key.file, key.block, merged[key])
	}
	return w.Flush()
}
