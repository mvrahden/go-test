# Delta for Cart

## ADDED Requirements

### Requirement: Gift wrap
The cart SHALL wrap gifts.

#### Scenario: A wrapped gift
- **WHEN** a gift is wrapped
- **THEN** adds the wrapping

## MODIFIED Requirements

### Requirement: Totals
The cart SHALL total its lines and its wrapping.

#### Scenario: A total with wrapping
- **WHEN** a wrapped gift is totalled
- **THEN** counts the wrapping

## REMOVED Requirements

### Requirement: Refunds
**Reason**: refunds moved to payments.

#### Scenario: A removed refund
- **WHEN** a refund is asked for
- **THEN** returns the money
