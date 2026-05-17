# auth — Token Validation & Error Handling

Demonstrates error assertions, parameterized testing, and focus/exclude modifiers
using a JWT-like token validator, password policy, and email format checker.

## Structure

- **validator.go** — Token validator, password policy, email validation
- **suite_test.go** — `TokenValidatorTestSuite`, `F_TokenValidatorTestSuite` (focused), `X_DeprecatedOAuthTestSuite` (excluded)
- **suite_ext_test.go** — `TokenValidatorTestSuite` (external package variant)

## Features

`ErrorIs` · `ErrorAs` · `ErrorContains` · `Panics` · `Must` · `Regexp` · `t.Each` · `gotest.Each` · `F_` focus · `X_` exclude
