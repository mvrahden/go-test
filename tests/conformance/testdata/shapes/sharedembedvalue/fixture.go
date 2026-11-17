package sharedembedvalue

import "context"

// Endpoint is a value the suites would read.
type Endpoint string

// EndpointSharedFixture embeds it.
type EndpointSharedFixture struct {
	Endpoint
}

func (f *EndpointSharedFixture) BeforeAll(ctx context.Context) error {
	f.Endpoint = "http://x"
	return nil
}
