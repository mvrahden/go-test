# auth — Token Validation & Error Handling

Demonstrates error assertions, parameterized testing, and the exclude modifier
using a JWT-like token validator, password policy, and email format checker.

## Structure

- **validator.go** — Token validator, password policy, email validation
- **suite_test.go** — `TokenValidatorTestSuite`, `X_DeprecatedOAuthTestSuite` (excluded)
- **suite_ext_test.go** — `TokenValidatorTestSuite` (external package variant)

## Features

`ErrorIs` · `ErrorAs` · `ErrorContains` · `Panics` · `Must` · `Regexp` · `t.Each` · `gotest.Each` · `X_` exclude
