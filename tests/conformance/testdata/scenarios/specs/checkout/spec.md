# Checkout Specification

## Purpose
What checkout promises, judged by a run.

## Requirements

### Requirement: Payment
Checkout SHALL take payment.

#### Scenario: A paid order
- **WHEN** an order is paid
- **THEN** marks it paid

#### Scenario: A failed payment
- **WHEN** a payment fails
- **THEN** keeps the order open

#### Scenario: A gateway that is down
- **WHEN** the gateway is down
- **THEN** retries later

#### Scenario: A bank that is offline
- **WHEN** the bank is offline
- **THEN** queues the order

#### Scenario: A receipt
- **WHEN** a receipt is printed
- **THEN** prints the total

#### Scenario: Parcels
- **WHEN** a parcel is weighed
- **THEN** prices it by weight
