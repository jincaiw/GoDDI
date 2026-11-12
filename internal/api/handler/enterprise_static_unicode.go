package handler

import (
	"encoding/json"
	"strconv"
	"strings"
)

// validJSONSurrogates prevents encoding/json from silently replacing unpaired
// surrogate escapes. Full JSON syntax is still validated by the decoder.
func validJSONSurrogates(b []byte) bool {
	in := false
	for i := 0; i < len(b); i++ {
		if !in {
			if b[i] == '"' {
				in = true
			}
			continue
		}
		if b[i] == '"' {
			in = false
			continue
		}
		if b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		n, ok := hexCode(b[i+1:])
		if !ok {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			m, ok := hexCode(b[i+3:])
			if !ok || m < 0xdc00 || m > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
func hexCode(b []byte) (uint16, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var n uint16
	for _, c := range b[:4] {
		var x byte
		switch {
		case c >= '0' && c <= '9':
			x = c - '0'
		case c >= 'a' && c <= 'f':
			x = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			x = c - 'A' + 10
		default:
			return 0, false
		}
		n = n<<4 | uint16(x)
	}
	return n, true
}

// validHoldSeconds applies JSON Schema's mathematical integer semantics,
// including 1.0 and 1e0, without float rounding or arbitrary-size arithmetic.
func validHoldSeconds(raw json.RawMessage) bool {
	s := string(raw)
	if len(s) == 0 || s[0] == '-' || s[0] == '"' || s[0] == 'n' || s[0] == 't' || s[0] == 'f' {
		return false
	}
	exp := int64(0)
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		v, err := strconv.ParseInt(s[i+1:], 10, 64)
		if err != nil || v > 2<<20 || v < -(2<<20) {
			return false
		}
		exp = v
		s = s[:i]
	}
	frac := 0
	if i := strings.IndexByte(s, '.'); i >= 0 {
		frac = len(s) - i - 1
		s = s[:i] + s[i+1:]
	}
	digits := strings.TrimLeft(s, "0")
	if digits == "" {
		return false
	}
	shift := exp - int64(frac)
	if shift < 0 {
		n := -shift
		if n > int64(len(digits)) {
			return false
		}
		cut := len(digits) - int(n)
		if strings.Trim(digits[cut:], "0") != "" {
			return false
		}
		digits = digits[:cut]
		if digits == "" {
			return false
		}
	} else if shift > 5 {
		return false
	} else {
		digits += strings.Repeat("0", int(shift))
	}
	digits = strings.TrimLeft(digits, "0")
	if len(digits) > 5 || digits == "" {
		return false
	}
	n, err := strconv.Atoi(digits)
	return err == nil && n >= 1 && n <= 86400
}
