// Package soaserial implements RFC 1982 arithmetic for 32-bit SOA serials.
// It does not provide persistent lineage, publication, or authorization.
// Callers must separately verify historical serial/content bindings and prove
// continuity with retained peers and recovery state before advancing a serial.
package soaserial

import "errors"

var (
	ErrAmbiguous = errors.New("soaserial: serials differ by exactly half the sequence space")
	ErrIncrement = errors.New("soaserial: increment must be between 1 and 2^31-1")
)

const half uint32 = 1 << 31

// Compare returns -1 if a precedes b, 0 if equal, or 1 if a follows b.
// Values separated by exactly 2^31 have no defined ordering. The relation is
// not transitive over arbitrary serials distributed around the entire ring.
func Compare(a, b uint32) (int, error) {
	delta := b - a
	if delta == 0 {
		return 0, nil
	}
	if delta == half {
		return 0, ErrAmbiguous
	}
	if delta < half {
		return -1, nil
	}
	return 1, nil
}

// Next advances the serial modulo 2^32 by an explicitly bounded increment.
func Next(serial, increment uint32) (uint32, error) {
	if increment == 0 || increment >= half {
		return 0, ErrIncrement
	}
	return serial + increment, nil
}
