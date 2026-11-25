# Cart Specification

## Purpose
What the cart promises, for the scenarios conformance suites to bind.

## Requirements

### Requirement: Totals
The cart SHALL total its lines.

#### Scenario: An empty cart
- **WHEN** The cart is `empty`.
- **THEN** totals zero
- **AND** lists no line

#### Scenario: A member's discount
- **GIVEN** a member
- **WHEN** a discount applies
- **THEN** the total drops

#### Scenario: A refund
- **WHEN** a refund is asked for
- **THEN** returns the money

#### Scenario: A coupon
- **WHEN** a coupon is applied
- **THEN** the total drops by its value
- **AND** the coupon is used up

#### Scenario: 空のカート
- **WHEN** カートを空にする
- **THEN** 合計を表示する

#### Scenario: 満杯のカート
- **WHEN** カートを満たす
- **THEN** 合計を表示する

#### Scenario: Rows of a table
- **WHEN** rows are read
- **THEN** reads each row

#### Scenario: A parked check
- **WHEN** the check is parked
- **THEN** still holds

### Requirement: Ambiguity
The cart SHALL be stated once.

#### Scenario: Stock counted twice
- **WHEN** stock is counted
- **THEN** counts each item

#### Scenario: A price
- **WHEN** a price is shown
- **THEN** shows the currency

#### Scenario: Sizes
- **WHEN** a size is chosen
- **THEN** fits the box
