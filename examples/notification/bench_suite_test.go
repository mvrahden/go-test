package notification

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type NotificationDispatchBenchTestSuite struct {
	dispatcher *dispatcher
}

func (s *NotificationDispatchBenchTestSuite) BeforeEach(t *gotest.T) {
	s.dispatcher = newDispatcher()
}

func (s *NotificationDispatchBenchTestSuite) BenchmarkDispatch(b *gotest.B) {
	for b.Loop() {
		s.dispatcher.Send(Notification{
			To:      "bench@example.com",
			Subject: "Benchmark",
		})
	}
}
