package ledger

import (
	"context"
	"errors"
	"os"
)

// LedgerSharedFixture starts one ledger for the whole run. The server lives
// in the fixture process; each suite process gets the URL and its own client.
type LedgerSharedFixture struct {
	URL    string  // transfer field: where the ledger listens
	PID    int     // transfer field: the process that holds it
	Client *Client // local field: assigned in Hydrate

	srv *Server // fixture process only
}

// BeforeAll starts the ledger in the fixture process.
func (f *LedgerSharedFixture) BeforeAll(ctx context.Context) error {
	srv, err := Start()
	if err != nil {
		return err
	}
	f.srv, f.URL, f.PID = srv, srv.URL(), os.Getpid()
	return nil
}

// AfterAll stops the ledger; it also runs after a BeforeAll that failed.
func (f *LedgerSharedFixture) AfterAll(ctx context.Context) error {
	if f.srv == nil {
		return nil
	}
	return f.srv.Close(ctx)
}

// Hydrate gives a suite process its own client for the ledger.
func (f *LedgerSharedFixture) Hydrate(ctx context.Context) error {
	if f.URL == "" {
		return errors.New("ledger: no URL; the fixture's BeforeAll did not run")
	}
	f.Client = NewClient(f.URL)
	return nil
}

// Dehydrate drops the client; it also runs after a Hydrate that failed.
func (f *LedgerSharedFixture) Dehydrate(ctx context.Context) error {
	if f.Client != nil {
		f.Client.Close()
	}
	return nil
}
