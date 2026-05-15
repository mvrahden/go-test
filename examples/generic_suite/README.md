# Generic Suite

Type-parameterized suites.

A generic suite like `GenericTestSuite[T]` is instantiated via type aliases
(e.g. `StringTestSuite = GenericTestSuite[string]`). Each alias becomes its
own test suite with the shared behavior specialized to the type argument.
