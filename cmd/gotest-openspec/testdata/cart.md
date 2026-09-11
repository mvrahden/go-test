# cart Specification

## Purpose
A shopping cart that tracks items, quantities, discounts and checkout.

## Requirements

### Requirement: Adding items
The cart SHALL track each added item with its quantity and line total.

#### Scenario: increases the item count
- **WHEN** a single item is added to an empty cart
- **THEN** the item count is 1

#### Scenario: merges the quantities
- **WHEN** the same item is added twice
- **THEN** the quantities are merged into one entry

### Requirement: Discounts
The cart SHALL apply percentage discounts and never charge a negative total.

#### Scenario: the discount exceeds 100 percent clamps to zero
- **WHEN** a discount over 100% is applied
- **THEN** the total is zero

#### Scenario: applies a coupon code
- **WHEN** a valid coupon code is entered
- **THEN** the coupon's discount is applied

### Requirement: Checkout
The cart SHALL refuse to check out when empty.

#### Scenario: the cart is empty returns an error
- **WHEN** checkout is called on an empty cart
- **THEN** an error is returned
