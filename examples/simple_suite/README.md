# Simple Suite

Basic suite pattern — the starting point for new users.

A suite is a struct whose methods define lifecycle hooks and test cases.
`BeforeEach` runs before every test method, providing fresh state per test.
Test methods receive `*gotest.T` and use framework assertions.

Both internal (`ptest`) and external (`pxtest`) test packages are shown.
