package refactor

// InsertFuzzAdd is insertFuzzAdd for a file without a path.
func InsertFuzzAdd(src []byte, suiteName, methodName string, argExprs []string, types TypeNames) (edited []byte, found bool, err error) {
	return insertFuzzAdd("", src, suiteName, methodName, argExprs, types)
}
