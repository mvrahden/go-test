package sample

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
