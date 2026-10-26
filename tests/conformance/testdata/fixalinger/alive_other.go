//go:build !unix

package fixalinger

// alive is false where the suite starts no child.
func alive(int) bool { return false }
