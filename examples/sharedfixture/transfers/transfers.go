// Package transfers moves money between ledger accounts.
package transfers

import (
	"context"

	"github.com/mvrahden/go-test/examples/sharedfixture/ledger"
)

// Move takes cents from one account and gives them to another.
func Move(ctx context.Context, c *ledger.Client, from, to string, cents int64) error {
	if err := c.Withdraw(ctx, from, cents); err != nil {
		return err
	}
	return c.Deposit(ctx, to, cents)
}
