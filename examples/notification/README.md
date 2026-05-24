# notification — Async Delivery & Temporal Assertions

Demonstrates async testing with temporal assertions, JSON comparison, and snapshot testing using a notification dispatcher that processes messages asynchronously.

## Structure

- **dispatcher.go** — Async notification dispatcher with delivery tracking
- **suite_test.go** — `NotificationServiceTestSuite` with 6 test methods

## Features

`gotest.Eventually` · `gotest.Consistently` · `JSONEq` · `gotest.MatchSnapshot` · `TimeIsNow` · `TimeWithin` · `NewTWithDeadline`
