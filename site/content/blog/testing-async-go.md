---
title: "Testing Async Code in Go Without time.Sleep"
date: 2026-07-16
description: "Go async tests without time.Sleep: Eventually and Consistently separate how long to wait from how often to check, so tests stay fast and stop flaking."
tags: ["Patterns"]
keywords: ["go test async", "go eventually assertion", "time.sleep in tests flaky", "go polling test"]
cta_text: "Replace time.Sleep with Eventually in your next test."
---

Most Go code that tests async behavior uses `time.Sleep`. It's slow, flaky, or both. The real problem is that sleeping conflates two separate concerns: how long to wait and how often to check. This post shows how to separate them with `Eventually` and `Consistently`, and why that separation makes async tests both faster and more reliable.

## The sleep problem

Suppose you have a notification dispatcher that processes messages in a background goroutine. You send a notification and want to assert that it was delivered. The most common approach:

```go {title="dispatcher_test.go"}
func TestDeliverNotification(t *testing.T) {
    dispatcher := newDispatcher()
    dispatcher.Send(Notification{
        To:      "user@example.com",
        Subject: "Welcome",
        Body:    "Hello, welcome aboard!",
    })

    time.Sleep(500 * time.Millisecond)

    if dispatcher.DeliveryCount() != 1 {
        t.Errorf("expected 1 delivery, got %d", dispatcher.DeliveryCount())
    }
}
```

This test has two problems that pull in opposite directions:

- **Too short and it flakes.** If the dispatcher takes 600ms under CI load, the 500ms sleep isn't enough. The test fails, but not because anything is broken.
- **Too long and it drags.** You could sleep for 5 seconds "just to be safe," but then every run of this test wastes 4+ seconds waiting for nothing. Multiply by a test suite with dozens of async checks and you've added minutes to your pipeline.

The deeper issue is that the amount of sleep has nothing to do with the assertion. The assertion is "delivery count is 1." The sleep is a guess about how long the dispatcher needs. These are separate concerns, but `time.Sleep` jams them into a single number.

## The polling pattern

Experienced Go developers often replace the sleep with a hand-rolled polling loop:

```go {title="dispatcher_test.go"}
func TestDeliverNotification(t *testing.T) {
    dispatcher := newDispatcher()
    dispatcher.Send(Notification{
        To:      "user@example.com",
        Subject: "Welcome",
    })

    deadline := time.After(500 * time.Millisecond)
    ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-deadline:
            t.Fatalf("timed out waiting for delivery")
        case <-ticker.C:
            if dispatcher.DeliveryCount() == 1 {
                return
            }
        }
    }
}
```

This is better. The test checks every 10ms and gives up after 500ms. If the dispatcher finishes in 20ms, the test returns in ~30ms instead of sleeping for the full 500. But it comes with its own problems:

- **Verbose.** Seven lines of polling infrastructure to assert one thing. The actual check (`DeliveryCount() == 1`) is buried inside a select loop.
- **Error handling is awkward.** You can't call `t.Fatal` inside a goroutine; it panics. So if you ever need to poll from a separate goroutine, this pattern breaks.
- **No standard pattern.** Every team reinvents it. Different timeout values, different tick intervals, different error messages. Some use channels, some use contexts, some use `time.Tick` (which leaks). There's no shared vocabulary.

## Eventually: poll until true

`gotest.Eventually` extracts this polling loop into a single function call with three parameters:

- `waitFor`: the maximum time to wait (the deadline)
- `tick`: how often to check
- `fn`: a callback that receives a `*gotest.R` (poll recorder) and runs assertions against it

If the callback succeeds on any tick (no assertion failures), `Eventually` returns immediately. If the deadline passes without a successful tick, it reports the last failure message to the test runner.

The vocabulary here has prior art worth crediting: gomega's `Eventually` and `Consistently` established this pattern in the Go ecosystem, and testify offers `require.Eventually` for the polling case. gotest's version is distinguished by the details the examples below rely on — the `*gotest.R` recorder that lets ordinary assertions run inside polling callbacks, and the explicit `waitFor`/`tick` separation.

Here's the notification dispatcher test rewritten with `Eventually`, using [BDD-style structure]({{< ref "/blog/readable-tests-with-bdd" >}}):

```go {title="dispatcher_test.go"}
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
```

The intent is immediately clear: poll every 10ms, give up after 500ms, and assert that the delivery count equals 1. No channels, no select, no ticker cleanup. The polling infrastructure is handled for you.

## The `*gotest.R` recorder

You might have noticed that the callback receives `poll *gotest.R` instead of `t *gotest.T`. This is the key design decision that makes polling work with assertion functions.

The problem: calling `t.Fatal` (or any assertion that calls `FailNow`) inside a polling loop would kill the test on the first failure. But the whole point of polling is that early failures are expected. The condition isn't true *yet*.

`*gotest.R` solves this by acting as an assertion recorder. It implements the same interface as `*testing.T` for assertion purposes (`Errorf` and `FailNow`), but instead of propagating failures to the test runner, it captures them:

- `Errorf` records the failure message without stopping execution.
- `FailNow` marks the recorder as failed and calls `runtime.Goexit` to stop the current goroutine, but does not propagate to the test runner.

On each tick, `Eventually` runs the callback with a fresh `*R` in a dedicated goroutine (because `FailNow` calls `runtime.Goexit`, which only terminates the current goroutine). If the `*R` is not marked as failed after the callback returns, the condition has been met and `Eventually` returns. If it is marked as failed, the loop moves on to the next tick.

All gotest assertions work with `*gotest.R`. You use `gotest.Equal(poll, ...)` exactly as you would use `gotest.Equal(t, ...)`. Same API, different behavior under the hood.

## Consistently: assert stability

`gotest.Consistently` is the inverse of `Eventually`. Instead of polling until a condition becomes true, it asserts that a condition *stays* true for an entire duration:

- If the callback **fails** on any tick, the test fails immediately.
- If the duration passes without any failure, the test passes.

The primary use case is asserting that something did *not* happen. For example, verifying that an idle dispatcher doesn't produce phantom deliveries:

```go {title="dispatcher_test.go"}
func (s *NotificationServiceTestSuite) TestIdleDispatcher(t *gotest.T) {
    t.When("no notifications have been sent", func(t *gotest.T) {
        t.It("consistently reports zero deliveries", func(t *gotest.T) {
            gotest.Consistently(t, 200*time.Millisecond, 50*time.Millisecond, func(poll *gotest.R) {
                gotest.Equal(poll, 0, s.dispatcher.DeliveryCount())
            })
        })
    })
}
```

This checks every 50ms for 200ms that the delivery count remains zero. If a bug causes a spurious delivery, the test catches it on the tick where it happens rather than after a blind sleep.

`Consistently` is especially useful for race condition tests. If you're verifying that a mutex correctly prevents concurrent access, `Consistently` gives you repeated checks over time rather than a single snapshot that might miss the race.

## Eventually as a synchronization barrier

A powerful pattern is using `Eventually` to synchronize, then asserting properties on the result with normal test assertions. This separates "wait for readiness" from "check the result":

```go {title="dispatcher_test.go"}
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
    })
}
```

`Eventually` establishes that the delivery happened. Once it returns, the test proceeds with normal assertions on `t`, not `poll`. The `TimeIsNow` assertion uses the real test context because at this point the async operation is complete and there's nothing left to poll.

This pattern keeps your polling callbacks focused on a single condition (is the system ready?) and moves detailed assertions into `It` blocks where they belong. It also means that if the timestamp assertion fails, the error message comes from a regular assertion with a clear stack trace, not from a polling recorder.

If you're using [test fixtures]({{< ref "/blog/test-fixtures-in-go" >}}) with `BeforeEach`, the dispatcher setup happens in the fixture and the test methods stay focused on behavior. The `Eventually` call inside `When` acts as a synchronization point between the fixture's state and the assertions that follow.

## Choosing `waitFor` and `tick`

The two time parameters in `Eventually` and `Consistently` serve different purposes, and getting them right is what makes async tests both fast and reliable:

**`tick` controls responsiveness.** It determines how quickly the test notices that the condition has changed. For in-process async operations (goroutines, channels), 10-50ms is usually fast enough. For external services (databases, APIs), 100-500ms avoids overwhelming the service with requests.

**`waitFor` controls reliability.** It should be generous enough that the test never flakes in CI, even under load. A good rule of thumb is 5-10x the expected duration. If the dispatcher typically finishes in 50ms, a 500ms timeout gives ten times the headroom without making the happy path any slower.

This separation is the key insight. With `time.Sleep`, you pick one number that simultaneously controls how long the test waits (bad for speed) and how long it tolerates (bad for reliability). With `Eventually`, a generous `waitFor` makes the test reliable while a fast `tick` keeps it responsive. The test completes in `actual_duration + tick`, not `waitFor`.

> Sleep conflates the deadline and the check interval into a single value. Eventually separates them. That's the whole trick.

## When to use which

A quick decision guide:

- **Eventually** when you're waiting for something to happen: a message was delivered, a cache was populated, a goroutine finished its work.
- **Consistently** when you're verifying something does *not* happen: no messages leak, no state changes, a rate limiter holds.
- **Eventually then assert** when you need to wait for readiness and then check detailed properties. Use `Eventually` as the synchronization barrier, then switch to regular assertions on `t`.
- **Plain assertions** when the operation is synchronous. Don't reach for `Eventually` when a direct `gotest.Equal(t, ...)` would do.

Both `Eventually` and `Consistently` accept any value that implements `Errorf` and `FailNow` as their first argument. This means they work with both `*gotest.T` (in suite tests) and `*testing.T` (in standalone tests). You don't need to adopt the full gotest suite system to use them.

## Next steps

The [reference docs]({{< ref "/reference" >}}) cover the full assertion API, including every function that works with `*gotest.R` in polling callbacks. If you're new to gotest's suite system and the `BeforeEach`/`When`/`It` patterns used in the examples above, [Readable Go Tests with BDD-Style Subtests]({{< ref "/blog/readable-tests-with-bdd" >}}) walks through the structure in detail. For fixture lifecycle management, see [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}).
