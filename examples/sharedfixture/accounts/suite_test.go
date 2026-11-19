package accounts

import (
	"os"

	"github.com/mvrahden/go-test/examples/sharedfixture/ledger"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AccountsTestSuite struct {
	Ledger *ledger.LedgerSharedFixture
}

func (s *AccountsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AccountsTestSuite) TestOpen(t *gotest.T) {
	t.When("an account is opened with a deposit", func(w *gotest.T) {
		id := "accounts/" + w.Name()
		gotest.NoError(w, Open(w.Context(), s.Ledger.Client, id, 500))
		w.It("holds the deposit", func(it *gotest.T) {
			got, err := s.Ledger.Client.Balance(it.Context(), id)
			gotest.NoError(it, err)
			gotest.Equal(it, int64(500), got)
		})
	})
}

func (s *AccountsTestSuite) TestTheLedgerIsShared(t *gotest.T) {
	t.It("runs in the fixture process, not in this suite's", func(it *gotest.T) {
		gotest.NotZero(it, s.Ledger.PID)
		gotest.NotEqual(it, os.Getpid(), s.Ledger.PID)
	})
}
