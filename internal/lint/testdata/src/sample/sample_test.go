package sample

import "testing"

type UserServiceTestSuite struct{}

func (s *UserServiceTestSuite) TestCreate()    {}
func (s *UserServiceTestSuite) BeforeAll()     {}
func (s *UserServiceTestSuite) AfterAll()      {}
func (s *UserServiceTestSuite) BeforreAll()    {} // want `method BeforreAll on suite UserServiceTestSuite is similar to lifecycle hook BeforeAll`
func (s UserServiceTestSuite) TestByValue()    {} // want `suite method UserServiceTestSuite\.TestByValue should use a pointer receiver`
func (s *UserServiceTestSuite) F_TestFocused() {} // want `focused method UserServiceTestSuite\.F_TestFocused should not be committed`

type F_OrderTestSuite struct{} // want `focused suite F_OrderTestSuite should not be committed`

func (s *F_OrderTestSuite) TestList() {}

type PaymentTestSuite struct{} // want `suite PaymentTestSuite has BeforeAll but no AfterAll`

func (s *PaymentTestSuite) TestCharge() {}
func (s *PaymentTestSuite) BeforeAll()  {}

// X_ prefix is allowed — skip markers don't hide other tests behind a green CI run.
type X_InactiveTestSuite struct{}

func (s *X_InactiveTestSuite) TestSkipped() {}

// Generic suite with single type param — pointer receiver must be recognized.
type TypedTestSuite[T any] struct{}

func (s *TypedTestSuite[T]) TestTyped()                 {}
func (s TypedTestSuite[T]) TestTypedByValue()            {} // want `suite method TypedTestSuite\.TestTypedByValue should use a pointer receiver`
func (s *TypedTestSuite[T]) F_TestFocusedGeneric()       {} // want `focused method TypedTestSuite\.F_TestFocusedGeneric should not be committed`

// Generic suite with multiple type params — pointer receiver must be recognized.
type PairTestSuite[K comparable, V any] struct{}

func (s *PairTestSuite[K, V]) TestPair()        {}
func (s PairTestSuite[K, V]) TestPairByValue()   {} // want `suite method PairTestSuite\.TestPairByValue should use a pointer receiver`

func TestStdlib(t *testing.T) {} // want `stdlib test TestStdlib — consider using a gotest suite`
