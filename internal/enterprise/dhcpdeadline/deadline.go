// Package dhcpdeadline computes a conservative DHCPv4 lease duration only.
// It does not authorize sending. The caller must verify committed lease identity,
// role, epoch, authoritative tail and fencing, and serialize revalidation and
// socket send with the required commit and send gates. Never cache a result for
// a later send. The caller must pass the fresh timeguard.Current result and error.
package dhcpdeadline

import (
	"errors"
	"math"

	"github.com/jasonwa/goddi/internal/enterprise/trustedtime"
)

var ErrUnavailable = errors.New("dhcpdeadline: no safe lease duration")

const nanosPerSecond int64 = 1_000_000_000

// Seconds calculates floor((expiryLower-now.Upper-clockMargin-deliveryMargin)/1s).
// All times are positive UTC Unix nanoseconds. expiryLower is the conservative
// earliest possible expiry of the already committed lease, not its upper bound.
// Margins are nonnegative nanoseconds in addition to uncertainty in now. A
// non-nil timeErr from timeguard, inverted bounds, insufficient remaining time,
// overflow, or the DHCP infinite-lease value rejects the result. No rounding or
// clipping may extend an existing commitment.
func Seconds(expiryLower int64, now trustedtime.Bounds, timeErr error, clockMargin, deliveryMargin int64) (uint32, error) {
	if timeErr != nil || expiryLower <= 0 || now.Lower <= 0 || now.Upper < now.Lower || clockMargin < 0 || deliveryMargin < 0 || expiryLower <= now.Upper {
		return 0, ErrUnavailable
	}
	remaining := expiryLower - now.Upper // Both operands are positive and ordered.
	if clockMargin >= remaining {
		return 0, ErrUnavailable
	}
	remaining -= clockMargin
	if deliveryMargin >= remaining {
		return 0, ErrUnavailable
	}
	remaining -= deliveryMargin
	seconds := remaining / nanosPerSecond
	// 0xffffffff is the DHCP infinite lease and must never be emitted.
	if seconds <= 0 || seconds >= int64(math.MaxUint32) {
		return 0, ErrUnavailable
	}
	return uint32(seconds), nil
}
