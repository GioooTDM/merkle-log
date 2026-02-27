package main

import (
	"testing"
)

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
