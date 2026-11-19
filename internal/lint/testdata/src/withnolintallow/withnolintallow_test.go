package withnolintallow

import "testing"

func TestA(t *testing.T) {} //nolint:nilaway // want `stdlib test TestA`

func TestB(t *testing.T) {} //nolint:other // want `stdlib test TestB` `unknown rule other in nolint comment`
