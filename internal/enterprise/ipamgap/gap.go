// Package ipamgap finds the first free address in a bounded interval.
// It does not reserve addresses; callers must revalidate under their write transaction.
package ipamgap

import (
	"bytes"
	"errors"
	"fmt"
)

var (
	ErrInvalid = errors.New("invalid address interval input")
	ErrBudget  = errors.New("scan budget exhausted; retry")
	ErrFull    = errors.New("address range is full")
)

// Address is a 16-byte network-order integer. IPv4 uses the last four bytes;
// its first twelve bytes must be zero. IPv4-mapped IPv6 is not IPv4 here.
type Address [16]byte

type Interval struct{ First, Last Address }

// First returns the first free address in the inclusive scope. used contains
// inclusive occupied/reserved intervals sorted by First, each intersecting scope;
// they may extend beyond scope and may overlap or be adjacent. The caller must
// fetch the complete ordered set intersecting scope from its scoped storage query.
// family must be 4 or 6, endpoints must match the representation above, First
// must not exceed Last, and budget must be nonnegative. budget limits the number
// of input intervals inspected, including one that proves a gap. ErrBudget means
// the answer is unknown and is retryable; ErrFull means all of scope was proved
// occupied. Invalid intervals and ordering are checked as they are inspected;
// uninspected input is the caller's responsibility. No address enumeration occurs.
func First(family int, scope Interval, used []Interval, budget int) (Address, error) {
	var zero Address
	if (family != 4 && family != 6) || budget < 0 || !valid(family, scope) {
		return zero, ErrInvalid
	}
	candidate := scope.First
	var previous Address
	for i, r := range used {
		if i >= budget {
			return zero, ErrBudget
		}
		if !valid(family, r) || (i > 0 && compare(r.First, previous) < 0) || compare(r.Last, scope.First) < 0 || compare(r.First, scope.Last) > 0 {
			return zero, fmt.Errorf("%w: interval %d", ErrInvalid, i)
		}
		previous = r.First
		if compare(r.Last, candidate) < 0 {
			continue
		}
		if compare(r.First, candidate) > 0 {
			return candidate, nil
		}
		if compare(r.Last, scope.Last) >= 0 {
			return zero, ErrFull
		}
		// r.Last is strictly below scope.Last, so increment cannot overflow.
		candidate = r.Last
		increment(&candidate)
	}
	return candidate, nil
}

func valid(family int, r Interval) bool {
	if compare(r.First, r.Last) > 0 {
		return false
	}
	if family == 4 {
		var prefix [12]byte
		return bytes.Equal(r.First[:12], prefix[:]) && bytes.Equal(r.Last[:12], prefix[:])
	}
	return true
}

func compare(a, b Address) int { return bytes.Compare(a[:], b[:]) }
func increment(a *Address) {
	for i := len(a) - 1; i >= 0; i-- {
		a[i]++
		if a[i] != 0 {
			return
		}
	}
}
