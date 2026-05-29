package sample

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type UserServiceTestSuite struct{}

func (s *UserServiceTestSuite) TestCreate()                {} // want `test method UserServiceTestSuite\.TestCreate has wrong signature`
func (s *UserServiceTestSuite) TestCorrectSig(t *gotest.T)                    {}
func (s *UserServiceTestSuite) TestContextualSig(t *gotest.T, ctx *struct{})  {}
func (s *UserServiceTestSuite) TestStdlibSig(t *testing.T)                    {}
func (s *UserServiceTestSuite) TestStdlibCtxSig(t *testing.T, ctx *struct{})  {}
func (s *UserServiceTestSuite) BeforeAll()                 {}
func (s *UserServiceTestSuite) AfterAll()                  {}
func (s *UserServiceTestSuite) X_AfterEach()               {} // want `X_ prefix on lifecycle hook UserServiceTestSuite\.X_AfterEach has no effect`
func (s *UserServiceTestSuite) BeforreAll()                {} // want `method BeforreAll on suite UserServiceTestSuite is similar to lifecycle hook BeforeAll`
func (s UserServiceTestSuite) TestByValue()                {} // want `suite method UserServiceTestSuite\.TestByValue should use a pointer receiver` `test method UserServiceTestSuite\.TestByValue has wrong signature`
func (s *UserServiceTestSuite) F_TestFocused()             {} // want `focused method UserServiceTestSuite\.F_TestFocused should not be committed` `test method UserServiceTestSuite\.F_TestFocused has wrong signature`

type F_OrderTestSuite struct{} // want `focused suite F_OrderTestSuite should not be committed`

func (s *F_OrderTestSuite) TestList() {} // want `test method F_OrderTestSuite\.TestList has wrong signature`

type PaymentTestSuite struct{} // want `suite PaymentTestSuite has BeforeAll but no AfterAll`

func (s *PaymentTestSuite) TestCharge() {} // want `test method PaymentTestSuite\.TestCharge has wrong signature`
func (s *PaymentTestSuite) BeforeAll()  {}

// X_ prefix is allowed — skip markers don't hide other tests behind a green CI run.
type X_InactiveTestSuite struct{}

func (s *X_InactiveTestSuite) TestSkipped() {} // want `test method X_InactiveTestSuite\.TestSkipped has wrong signature`

// Generic suite with single type param — pointer receiver must be recognized.
type TypedTestSuite[T any] struct{}

func (s *TypedTestSuite[T]) TestTyped()            {} // want `test method TypedTestSuite\.TestTyped has wrong signature`
func (s TypedTestSuite[T]) TestTypedByValue()      {} // want `suite method TypedTestSuite\.TestTypedByValue should use a pointer receiver` `test method TypedTestSuite\.TestTypedByValue has wrong signature`
func (s *TypedTestSuite[T]) F_TestFocusedGeneric() {} // want `focused method TypedTestSuite\.F_TestFocusedGeneric should not be committed` `test method TypedTestSuite\.F_TestFocusedGeneric has wrong signature`

// Generic suite with multiple type params — pointer receiver must be recognized.
type PairTestSuite[K comparable, V any] struct{}

func (s *PairTestSuite[K, V]) TestPair()       {} // want `test method PairTestSuite\.TestPair has wrong signature`
func (s PairTestSuite[K, V]) TestPairByValue() {} // want `suite method PairTestSuite\.TestPairByValue should use a pointer receiver` `test method PairTestSuite\.TestPairByValue has wrong signature`

func TestStdlib(t *testing.T) {} // want `stdlib test TestStdlib — consider using a gotest suite`
