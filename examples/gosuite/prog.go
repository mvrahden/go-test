package gosuite

type Unit struct {
	count int
}

func NewUnit() *Unit {
	return &Unit{}
}

func (u *Unit) DoSomething() string {
	defer func() { u.count = (u.count + 1) % 5 }()
	list := []string{"hello", "world", "foo", "bar", "baz"}
	return list[u.count]
}
