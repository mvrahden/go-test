// Package plain is run only by the plain binary.
package plain

// Bye says goodbye to name.
func Bye(name string) string {
	if name == "" {
		return "bye, nobody"
	}
	return "bye, " + name
}
