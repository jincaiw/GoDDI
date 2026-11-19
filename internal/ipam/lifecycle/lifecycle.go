// Package lifecycle names the states of IPAM parent resources.
package lifecycle

const (
	Active  = "active"
	Retired = "retired"
)

// Valid reports whether s is a stored lifecycle state.
func Valid(s string) bool { return s == Active || s == Retired }
