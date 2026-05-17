package notification

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type NotificationServiceTestSuite struct {
	dispatcher *dispatcher
}

func (s *NotificationServiceTestSuite) BeforeEach(t *gotest.T) {
	s.dispatcher = newDispatcher()
}

func (s *NotificationServiceTestSuite) TestDeliverNotification(t *gotest.T) {
	t.When("a single notification is dispatched", func(t *gotest.T) {
		s.dispatcher.Send(Notification{
			To:       "user@example.com",
			Subject:  "Welcome",
			Body:     "Hello, welcome aboard!",
			Priority: PriorityNormal,
		})

		t.It("eventually delivers the message", func(t *gotest.T) {
			t.Eventually(500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.T) {
				gotest.Equal(poll, 1, s.dispatcher.DeliveryCount())
			})
		})
	})
}

func (s *NotificationServiceTestSuite) TestBatchDelivery(t *gotest.T) {
	t.When("multiple notifications are sent at once", func(t *gotest.T) {
		for i := range 3 {
			s.dispatcher.Send(Notification{
				To:      "team@example.com",
				Subject: fmt.Sprintf("Update #%d", i+1),
			})
		}

		t.It("eventually delivers all messages", func(t *gotest.T) {
			gotest.Eventually(t, func() bool {
				return s.dispatcher.DeliveryCount() == 3
			}, 500*time.Millisecond, 10*time.Millisecond)
		})
	})
}

func (s *NotificationServiceTestSuite) TestIdleDispatcher(t *gotest.T) {
	t.When("no notifications have been sent", func(t *gotest.T) {
		t.It("consistently reports zero deliveries", func(t *gotest.T) {
			t.Consistently(200*time.Millisecond, 50*time.Millisecond, func(poll *gotest.T) {
				gotest.Equal(poll, 0, s.dispatcher.DeliveryCount())
			})
		})

		t.It("validates using the function-level form", func(t *gotest.T) {
			gotest.Consistently(t, func() bool {
				return s.dispatcher.DeliveryCount() == 0
			}, 200*time.Millisecond, 50*time.Millisecond)
		})
	})
}

func (s *NotificationServiceTestSuite) TestDeliveryTimestamp(t *gotest.T) {
	t.When("a notification is delivered", func(t *gotest.T) {
		before := time.Now()
		s.dispatcher.Send(Notification{To: "user@example.com", Subject: "Timestamp check"})

		t.Eventually(500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.T) {
			gotest.Equal(poll, 1, s.dispatcher.DeliveryCount())
		})
		delivered := s.dispatcher.Deliveries()[0]

		t.It("records a recent timestamp", func(t *gotest.T) {
			gotest.TimeIsNow(t, delivered.DeliveredAt, 2*time.Second)
		})

		t.It("records the timestamp close to send time", func(t *gotest.T) {
			gotest.TimeWithin(t, before, delivered.DeliveredAt, 2*time.Second)
		})
	})
}

func (s *NotificationServiceTestSuite) TestNotificationPayload(t *gotest.T) {
	t.When("a high-priority notification is delivered", func(t *gotest.T) {
		s.dispatcher.Send(Notification{
			To:       "admin@example.com",
			Subject:  "System Alert",
			Body:     "CPU usage exceeded threshold",
			Priority: PriorityHigh,
		})

		t.Eventually(500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.T) {
			gotest.Equal(poll, 1, s.dispatcher.DeliveryCount())
		})
		delivered := s.dispatcher.Deliveries()[0]

		t.It("serializes to the expected JSON", func(t *gotest.T) {
			actual, _ := json.Marshal(delivered.Notification)
			gotest.JSONEq(t,
				`{"to":"admin@example.com","subject":"System Alert","body":"CPU usage exceeded threshold","priority":2}`,
				string(actual),
			)
		})

		t.It("matches the delivery summary snapshot", func(t *gotest.T) {
			t.MatchSnapshot(formatSummary(delivered))
		})
	})
}

func (s *NotificationServiceTestSuite) TestDeadlineContext(t *gotest.T) {
	t.When("a deadline is configured for the test", func(t *gotest.T) {
		dt := gotest.NewTWithDeadline(t.T(), 5*time.Second)
		ctx := dt.Context()

		t.It("exposes the deadline on the context", func(t *gotest.T) {
			deadline, ok := ctx.Deadline()
			gotest.True(t, ok)
			gotest.True(t, deadline.After(time.Now()))
		})
	})
}
