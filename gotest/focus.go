package gotest

// resolveFocus applies focus/exclude logic to the registered tests and describes.
// If any item is focused, all non-focused items become excluded.
// Already-excluded items stay excluded regardless.
func resolveFocus(tests []testEntry, describes []describeEntry) ([]testEntry, []describeEntry) {
	hasFocused := false
	for _, t := range tests {
		if t.focused {
			hasFocused = true
			break
		}
	}
	if !hasFocused {
		for _, d := range describes {
			if d.focused {
				hasFocused = true
				break
			}
		}
	}

	if !hasFocused {
		return tests, describes
	}

	resolvedTests := make([]testEntry, len(tests))
	copy(resolvedTests, tests)
	for i := range resolvedTests {
		if !resolvedTests[i].focused {
			resolvedTests[i].excluded = true
		}
	}

	resolvedDescs := make([]describeEntry, len(describes))
	copy(resolvedDescs, describes)
	for i := range resolvedDescs {
		if !resolvedDescs[i].focused {
			resolvedDescs[i].excluded = true
		}
	}

	return resolvedTests, resolvedDescs
}
