package transfers

import (
	"github.com/mvrahden/go-test/examples/sharedfixture/ledger"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type TransfersTestSuite struct {
	Ledger *ledger.LedgerSharedFixture
}

func (s *TransfersTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *TransfersTestSuite) TestMove(t *gotest.T) {
	t.When("the source holds enough", func(w *gotest.T) {
		from, to := "transfers/"+w.Name()+"/from", "transfers/"+w.Name()+"/to"
		gotest.NoError(w, s.Ledger.Client.Deposit(w.Context(), from, 300))
		gotest.NoError(w, Move(w.Context(), s.Ledger.Client, from, to, 200))
		w.It("takes the amount from the source", func(it *gotest.T) {
			got, err := s.Ledger.Client.Balance(it.Context(), from)
			gotest.NoError(it, err)
			gotest.Equal(it, int64(100), got)
		})
		w.It("gives it to the target", func(it *gotest.T) {
			got, err := s.Ledger.Client.Balance(it.Context(), to)
			gotest.NoError(it, err)
			gotest.Equal(it, int64(200), got)
		})
	})
	t.When("the source holds too little", func(w *gotest.T) {
		from, to := "transfers/"+w.Name()+"/from", "transfers/"+w.Name()+"/to"
		w.It("returns ErrInsufficient", func(it *gotest.T) {
			gotest.ErrorIs(it, Move(it.Context(), s.Ledger.Client, from, to, 1), ledger.ErrInsufficient)
		})
	})
}
