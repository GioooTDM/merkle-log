package hashutil

import (
	"strings"
	"testing"
)

func TestParsePayloadHash(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "valid with prefix", input: "hex:" + strings.Repeat("ab", 32), want: strings.Repeat("ab", 32)},
		{name: "valid uppercase trimmed", input: "  HEX:" + strings.Repeat("AB", 32) + "  ", want: strings.Repeat("ab", 32)},
		{name: "empty rejected", input: "", wantErr: true},
		{name: "invalid chars", input: "hex:zz", wantErr: true},
		{name: "invalid length", input: "hex:abcd", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePayloadHashValue(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParsePayloadHashValue() = %q, want %q", got, tc.want)
			}
		})
	}
}
