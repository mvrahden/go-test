// Package ledger is an in-memory account ledger served over HTTP, the
// resource the example's shared fixture starts once for a whole run.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// ErrInsufficient reports a withdrawal larger than the balance.
var ErrInsufficient = errors.New("insufficient funds")

// Server is an in-memory ledger behind HTTP on a loopback port.
type Server struct {
	listener net.Listener
	srv      *http.Server

	mu       sync.Mutex
	balances map[string]int64
}

// Start listens on 127.0.0.1:0 and serves until Close.
func Start() (*Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{listener: l, balances: map[string]int64{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /deposit", s.deposit)
	mux.HandleFunc("POST /withdraw", s.withdraw)
	mux.HandleFunc("GET /balance", s.balance)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(l) }() // Serve returns ErrServerClosed after Close
	return s, nil
}

// URL is where the ledger listens.
func (s *Server) URL() string { return "http://" + s.listener.Addr().String() }

// Close stops the server, waiting for requests in flight.
func (s *Server) Close(ctx context.Context) error { return s.srv.Shutdown(ctx) }

func (s *Server) deposit(w http.ResponseWriter, r *http.Request) {
	account, cents, ok := parse(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances[account] += cents
}

func (s *Server) withdraw(w http.ResponseWriter, r *http.Request) {
	account, cents, ok := parse(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.balances[account] < cents {
		http.Error(w, ErrInsufficient.Error(), http.StatusConflict)
		return
	}
	s.balances[account] -= cents
}

func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprint(w, s.balances[r.URL.Query().Get("account")])
}

// parse reads the account and a positive amount of cents from the query.
func parse(w http.ResponseWriter, r *http.Request) (string, int64, bool) {
	q := r.URL.Query()
	cents, err := strconv.ParseInt(q.Get("cents"), 10, 64)
	if q.Get("account") == "" || err != nil || cents <= 0 {
		http.Error(w, "want account and a positive cents", http.StatusBadRequest)
		return "", 0, false
	}
	return q.Get("account"), cents, true
}

// Client talks to a Server.
type Client struct {
	base string
	http *http.Client
}

// NewClient returns a client for the ledger at url.
func NewClient(url string) *Client {
	return &Client{base: url, http: &http.Client{Transport: &http.Transport{}}}
}

// Deposit adds cents to the account.
func (c *Client) Deposit(ctx context.Context, account string, cents int64) error {
	_, err := c.do(ctx, http.MethodPost, "/deposit", account, cents)
	return err
}

// Withdraw takes cents from the account; ErrInsufficient when the balance is short.
func (c *Client) Withdraw(ctx context.Context, account string, cents int64) error {
	_, err := c.do(ctx, http.MethodPost, "/withdraw", account, cents)
	return err
}

// Balance reads the account's balance in cents.
func (c *Client) Balance(ctx context.Context, account string) (int64, error) {
	body, err := c.do(ctx, http.MethodGet, "/balance", account, 0)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(body, 10, 64)
}

// Close drops the client's idle connections.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) do(ctx context.Context, method, path, account string, cents int64) (string, error) {
	q := url.Values{"account": {account}}
	if cents != 0 {
		q.Set("cents", strconv.FormatInt(cents, 10))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	switch {
	case err != nil:
		return "", err
	case resp.StatusCode == http.StatusConflict:
		return "", ErrInsufficient
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("ledger: %s %s: %s", method, path, resp.Status)
	}
	return string(body), nil
}
