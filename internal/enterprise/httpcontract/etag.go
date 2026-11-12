// Package httpcontract contains standalone helpers for the management API.
// It does not perform authentication, authorization or database writes.
package httpcontract

import (
	"errors"
	"net/http"
	"strconv"
)

var (
	ErrIfMatchMissing   = errors.New("httpcontract: If-Match missing")
	ErrIfMatchInvalid   = errors.New("httpcontract: invalid If-Match")
	ErrRevisionMismatch = errors.New("httpcontract: revision mismatch")
	ErrInvalidRevision  = errors.New("httpcontract: invalid current revision")
)

// ParseIfMatch accepts the complete set of field values as received by the
// HTTP layer. A zero-length slice means absent; a present empty field is
// malformed. Callers must include every field line, not just Header.Get.
// Exactly one strong tag in the form "v<positive int64>" is accepted.
func ParseIfMatch(values []string) (int64, error) {
	if len(values) == 0 {
		return 0, ErrIfMatchMissing
	}
	if len(values) != 1 {
		return 0, ErrIfMatchInvalid
	}
	s := values[0]
	// The largest possible value has 19 digits, plus quotes and v.
	if len(s) < 4 || len(s) > 22 || s[0] != '"' || s[1] != 'v' || s[len(s)-1] != '"' {
		return 0, ErrIfMatchInvalid
	}
	digits := s[2 : len(s)-1]
	if digits[0] < '1' || digits[0] > '9' {
		return 0, ErrIfMatchInvalid
	}
	for i := 1; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, ErrIfMatchInvalid
		}
	}
	revision, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, ErrIfMatchInvalid
	}
	return revision, nil
}

// FormatETag renders a valid revision as a strong ETag.
func FormatETag(revision int64) (string, error) {
	if revision <= 0 {
		return "", ErrInvalidRevision
	}
	return `"v` + strconv.FormatInt(revision, 10) + `"`, nil
}

// CheckIfMatch parses and compares a precondition against the current revision.
// It returns zero status on success, or 428, 400, or 412 for client failures.
// An invalid stored revision is an internal error, reported as 500. Call this
// after authentication and idempotency receipt handling, within the write
// transaction when evaluating the current revision.
func CheckIfMatch(values []string, current int64) (int, error) {
	revision, err := ParseIfMatch(values)
	if err != nil {
		if errors.Is(err, ErrIfMatchMissing) {
			return http.StatusPreconditionRequired, err
		}
		return http.StatusBadRequest, err
	}
	if current <= 0 {
		return http.StatusInternalServerError, ErrInvalidRevision
	}
	if revision != current {
		return http.StatusPreconditionFailed, ErrRevisionMismatch
	}
	return 0, nil
}
