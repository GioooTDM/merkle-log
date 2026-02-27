package main

import (
	"strings"
	"testing"
)

// TODO: meglio spostarla in validation_test.go
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
			got, err := parsePayloadHashValue(tc.input)
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
				t.Fatalf("parsePayloadHashValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseIndexFromPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		prefix  string
		want    uint64
		wantErr bool
	}{
		{name: "valid index", path: "/get-entry/42", prefix: "/get-entry/", want: 42},
		{name: "zero index", path: "/get-entry/0", prefix: "/get-entry/", want: 0},
		{name: "missing index", path: "/get-entry/", prefix: "/get-entry/", wantErr: true},
		{name: "non-numeric index", path: "/get-entry/not-a-number", prefix: "/get-entry/", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIndexFromPath(tc.path, tc.prefix)
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
				t.Fatalf("parseIndexFromPath() = %d, want %d", got, tc.want)
			}
		})
	}
}
