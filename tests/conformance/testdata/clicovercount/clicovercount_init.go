package clicovercount

// initValue runs when the package initialises: once per suite process.
func initValue() int { return 7 }

var initialised = initValue()

// Initialised keeps initialised in use.
func Initialised() int { return initialised }
