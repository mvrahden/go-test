// Package accounts opens accounts on the ledger.
package accounts

import (
	"context"

	"github.com/mvrahden/go-test/examples/sharedfixture/ledger"
)

// Open creates an account with an opening deposit.
func Open(ctx context.Context, c *ledger.Client, id string, cents int64) error {
	return c.Deposit(ctx, id, cents)
}
