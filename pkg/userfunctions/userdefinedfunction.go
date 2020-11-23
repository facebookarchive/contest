package userdefinedfunction

// UserDefinedFunction Interface - Every UDF needs to implement at least these
// functions in order to get hook up within ConTest.
type UserDefinedFunction interface {
	// Name returns the name of the step
	Name() string
	// Run runs the test step. The test step is expected to be synchronous.
	Load() (error, map[string]interface{})
}
