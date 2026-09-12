package server

import (
	"bytes"
	"testing"
)

func TestParseRelayAgentInfo(t *testing.T) {
	tests := []struct {
		name          string
		payload       []byte
		wantCircuit   string
		wantRemote    string
		wantUnknown   int
		wantErrSubstr string
	}{
		{
			name:        "circuit and remote with unknown suboption",
			payload:     []byte{1, 3, 'p', '1', '/', 2, 3, 'r', '1', 'x', 9, 1, 0xff},
			wantCircuit: "p1/",
			wantRemote:  "r1x",
			wantUnknown: 1,
		},
		{
			name:          "truncated header",
			payload:       []byte{1},
			wantErrSubstr: "truncated sub-option header",
		},
		{
			name:          "truncated value",
			payload:       []byte{1, 4, 'p', '1'},
			wantErrSubstr: "exceeds remaining",
		},
		{
			name:          "empty circuit",
			payload:       []byte{1, 0},
			wantErrSubstr: "circuit-id sub-option is empty",
		},
		{
			name:          "missing circuit",
			payload:       []byte{2, 1, 'r'},
			wantErrSubstr: "circuit-id sub-option is required",
		},
		{
			name:          "duplicate circuit",
			payload:       []byte{1, 1, 'a', 1, 1, 'b'},
			wantErrSubstr: "duplicate circuit-id",
		},
		{
			name:          "duplicate remote",
			payload:       []byte{1, 1, 'a', 2, 1, 'b', 2, 1, 'c'},
			wantErrSubstr: "duplicate remote-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRelayAgentInfo(tt.payload)
			if tt.wantErrSubstr != "" {
				if err == nil || !bytes.Contains([]byte(err.Error()), []byte(tt.wantErrSubstr)) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRelayAgentInfo() error = %v", err)
			}
			if string(got.CircuitID) != tt.wantCircuit {
				t.Errorf("CircuitID = %q, want %q", got.CircuitID, tt.wantCircuit)
			}
			if string(got.RemoteID) != tt.wantRemote {
				t.Errorf("RemoteID = %q, want %q", got.RemoteID, tt.wantRemote)
			}
			if got.UnknownSubopts != tt.wantUnknown {
				t.Errorf("UnknownSubopts = %d, want %d", got.UnknownSubopts, tt.wantUnknown)
			}
		})
	}
}

func TestRelayAgentInfoLogValueIsBoundedHex(t *testing.T) {
	got := relayAgentInfoLogValue(bytes.Repeat([]byte{0xab}, 80))
	if len(got) != 128 {
		t.Fatalf("hex log value length = %d, want 128", len(got))
	}
	if got[:4] != "abab" {
		t.Fatalf("hex log value starts %q, want abab", got[:4])
	}
}
