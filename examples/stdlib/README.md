# Stdlib Compatibility

`*testing.T` support and co-existence with standard Go tests.

Suite methods can use `*testing.T` signatures instead of `*gotest.T`.
The generated code unwraps via `T()` automatically.
Framework assertions like `gotest.True` also accept `*testing.T`, so both stdlib and gotest styles can be mixed freely.
Regular stdlib tests co-exist alongside suites in the same package.
