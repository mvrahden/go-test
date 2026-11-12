package sharedembedvalue

import "github.com/mvrahden/go-test/pkg/gotest"

type EndpointTestSuite struct{ Endpoint *EndpointSharedFixture }

func (s *EndpointTestSuite) TestEndpoint(t *gotest.T) {
	gotest.Equal(t, Endpoint("http://x"), s.Endpoint.Endpoint)
}
