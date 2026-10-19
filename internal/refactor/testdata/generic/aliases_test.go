package generic

type IntContractTestSuite = ContractTestSuite[int]

type StringContractTestSuite = ContractTestSuite[string]

var _ = (*IntContractTestSuite).TestCopy
