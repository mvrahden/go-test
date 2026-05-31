package protocol

const (
	EnvSharedStateFile    = "GOTEST_SHARED_STATE_FILE"
	EnvTeardownBudgetFile = "GOTEST_TEARDOWN_BUDGET_FILE"
	EnvUpdateSnapshots    = "GOTEST_UPDATE_SNAPSHOTS"
)

func BudgetFilePath(binaryPath string) string {
	return binaryPath + ".budget"
}
