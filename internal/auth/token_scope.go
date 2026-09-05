package auth

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidTokenScope = errors.New("invalid token scope")

type tokenScopeGrant struct {
	Resource string
	Action   string
}

// ValidateTokenScope validates the token scope syntax.
//
// Empty scopes remain valid for backward compatibility and mean "unrestricted".
func ValidateTokenScope(scope string) error {
	_, err := parseTokenScope(scope)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTokenScope, err)
	}
	return nil
}

// TokenScopeAllows reports whether a token scope grants access to the
// requested resource/action.
//
// Empty scopes are treated as unrestricted so existing tokens remain usable.
// Invalid scopes are treated as not granting access.
func TokenScopeAllows(scope, resource, action string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return true
	}
	grants, err := parseTokenScope(scope)
	if err != nil {
		return false
	}
	for _, grant := range grants {
		if grant.Resource != "*" && grant.Resource != resource {
			continue
		}
		if grant.Action == "*" || grant.Action == action {
			return true
		}
	}
	return false
}

func parseTokenScope(scope string) ([]tokenScopeGrant, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return nil, nil
	}

	parts := strings.FieldsFunc(scope, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\t', ' ':
			return true
		default:
			return false
		}
	})

	grants := make([]tokenScopeGrant, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "*" {
			grants = append(grants, tokenScopeGrant{Resource: "*", Action: "*"})
			continue
		}

		resource, action, found := strings.Cut(part, ":")
		resource = strings.TrimSpace(strings.ToLower(resource))
		if !found {
			if resource == "" {
				return nil, fmt.Errorf("empty scope grant")
			}
			grants = append(grants, tokenScopeGrant{Resource: resource, Action: "*"})
			continue
		}

		action = strings.TrimSpace(strings.ToLower(action))
		if resource == "" || action == "" {
			return nil, fmt.Errorf("invalid scope grant %q", part)
		}
		grants = append(grants, tokenScopeGrant{Resource: resource, Action: action})
	}

	if len(grants) == 0 {
		return nil, fmt.Errorf("empty scope grant")
	}

	return grants, nil
}
