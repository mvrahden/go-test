//go:build !windows

package gotestrunner

type jobObject struct{}

func newJobObject() (*jobObject, error) { return nil, nil }
func (j *jobObject) assign(pid int) error { return nil }
func (j *jobObject) terminate(exitCode uint32) error { return nil }
func (j *jobObject) close() {}
