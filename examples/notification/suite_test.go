package notification

import (
	"encoding/json"
	"fmt"
	"strings"
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
			gotest.Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
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
			gotest.Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
				gotest.Equal(poll, 3, s.dispatcher.DeliveryCount())
			})
		})
	})
}

func (s *NotificationServiceTestSuite) TestIdleDispatcher(t *gotest.T) {
	t.When("no notifications have been sent", func(t *gotest.T) {
		t.It("consistently reports zero deliveries", func(t *gotest.T) {
			gotest.Consistently(t, 200*time.Millisecond, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.Equal(poll, 0, s.dispatcher.DeliveryCount())
			})
		})
	})
}

func (s *NotificationServiceTestSuite) TestDeliveryTimestamp(t *gotest.T) {
	t.When("a notification is delivered", func(t *gotest.T) {
		before := time.Now()
		s.dispatcher.Send(Notification{To: "user@example.com", Subject: "Timestamp check"})

		gotest.Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
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

		gotest.Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
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
			gotest.MatchSnapshot(t, formatSummary(delivered))
		})
	})
}

// TestTrimSpaceTable exercises the same strings.TrimSpace call FuzzTrim's
// callback invokes — gotest generate harvests its literal table rows as
// extra f.Add(...) seeds for FuzzNotificationServiceTestSuite_FuzzTrim.
func (s *NotificationServiceTestSuite) TestTrimSpaceTable(t *gotest.T) {
	type tc struct {
		Desc string
		In   string
		Want string
	}
	for t, c := range gotest.Each(t, []tc{
		{"leading and trailing spaces", "  hello  ", "hello"},
		{"already trimmed", "hello", "hello"},
		{"tabs and newlines", "\thello\n", "hello"},
	}) {
		t.It("trims to the expected result", func(t *gotest.T) {
			gotest.Equal(t, c.Want, strings.TrimSpace(c.In))
		})
	}
}

func (s *NotificationServiceTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		// Property: strings.TrimSpace is idempotent — trimming an
		// already-trimmed string is a no-op round-trip.
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// FuzzSummary is a struct-typed fuzz target: Notification is not one of the
// fifteen types Go's fuzzing engine accepts, so gotest fans it out into one
// engine argument per field and reassembles it before each execution. The
// seed below is a plain Go literal — F.Add explodes it through the same fan.
func (s *NotificationServiceTestSuite) FuzzSummary(f *gotest.F) {
	f.Add(Notification{To: "a@b.c", Subject: "welcome", Priority: PriorityHigh})
	gotest.Fuzz(f, func(t *gotest.T, n Notification) {
		out := formatSummary(delivery{Notification: n})
		// Property: the summary always reproduces the recipient and subject
		// verbatim, behind exactly one known priority label.
		gotest.Contains(t, out, n.Subject)
		gotest.Contains(t, out, n.To)
		gotest.Regexp(t, `^\[(LOW|NORMAL|HIGH)\] `, out)
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
