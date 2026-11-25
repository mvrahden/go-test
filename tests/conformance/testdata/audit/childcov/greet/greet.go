// Package greet is run only by the hello binary.
package greet

// Hello greets name.
func Hello(name string) string {
	if name == "" {
		return "hello, nobody"
	}
	return "hello, " + name
}
