package auth

import "testing"

func TestValidateTokenScope(t *testing.T) {
	tests := []struct {
		name    string
		scope   string
		wantErr bool
	}{
		{name: "empty", scope: "", wantErr: false},
		{name: "single grant", scope: "dns:read", wantErr: false},
		{name: "multiple grants", scope: "dns:read,ipam:write", wantErr: false},
		{name: "wildcard", scope: "*", wantErr: false},
		{name: "resource wildcard", scope: "dns:*", wantErr: false},
		{name: "invalid empty action", scope: "dns:", wantErr: true},
		{name: "invalid empty resource", scope: ":read", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenScope(tt.scope)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateTokenScope(%q) = nil, want error", tt.scope)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateTokenScope(%q) = %v, want nil", tt.scope, err)
			}
		})
	}
}

func TestTokenScopeAllows(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		resource string
		action   string
		want     bool
	}{
		{name: "empty scope unrestricted", scope: "", resource: "dns", action: "write", want: true},
		{name: "exact match", scope: "dns:read", resource: "dns", action: "read", want: true},
		{name: "resource wildcard", scope: "dns:*", resource: "dns", action: "delete", want: true},
		{name: "global wildcard", scope: "*", resource: "ipam", action: "write", want: true},
		{name: "multiple grants", scope: "dns:read,ipam:write", resource: "ipam", action: "write", want: true},
		{name: "denied other action", scope: "dns:read", resource: "dns", action: "delete", want: false},
		{name: "denied other resource", scope: "dns:read", resource: "ipam", action: "read", want: false},
		{name: "invalid scope fails closed", scope: "dns:", resource: "dns", action: "read", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TokenScopeAllows(tt.scope, tt.resource, tt.action); got != tt.want {
				t.Fatalf("TokenScopeAllows(%q, %q, %q) = %v, want %v", tt.scope, tt.resource, tt.action, got, tt.want)
			}
		})
	}
}
