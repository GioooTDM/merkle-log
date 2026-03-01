package event

import (
	"testing"
	"time"
)

func TestParseRFC3339orNano(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "valid RFC3339",
			input: "2026-01-10T09:00:00Z",
			want:  time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC),
		},
		{
			name:  "valid RFC3339Nano",
			input: "2026-01-10T09:00:00.123456789Z",
			want:  time.Date(2026, 1, 10, 9, 0, 0, 123456789, time.UTC),
		},
		{
			name:    "invalid format",
			input:   "10-01-2026",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRFC3339orNano(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("parseRFC3339orNano() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRequireUUIDv4(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid UUIDv4", value: "550e8400-e29b-41d4-a716-446655440000", wantErr: false},
		{name: "valid UUIDv4 with spaces", value: "  550e8400-e29b-41d4-a716-446655440000  ", wantErr: false},
		{name: "empty string", value: "", wantErr: true},
		{name: "not a UUID", value: "not-a-uuid", wantErr: true},
		{name: "UUIDv1 rejected", value: "6ba7b810-9dad-11d1-80b4-00c04fd430c8", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireUUIDv4("field", tc.value)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRequireUUIDv4Ptr(t *testing.T) {
	t.Run("nil pointer returns error", func(t *testing.T) {
		if err := requireUUIDv4Ptr("field", nil); err == nil {
			t.Fatal("expected error for nil pointer, got nil")
		}
	})

	t.Run("valid pointer delegates to requireUUIDv4", func(t *testing.T) {
		v := "550e8400-e29b-41d4-a716-446655440000"
		if err := requireUUIDv4Ptr("field", &v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid UUID pointer returns error", func(t *testing.T) {
		v := "not-a-uuid"
		if err := requireUUIDv4Ptr("field", &v); err == nil {
			t.Fatal("expected error for invalid UUID, got nil")
		}
	})
}

func TestValidatePayloadHash(t *testing.T) {
	validValue := "hex:" +
		"aabbccdd00112233445566778899aabb" + // 32 hex chars
		"ccddeeff00112233445566778899aabb" // 32 hex chars

	tests := []struct {
		name    string
		ph      addEventPayloadHash
		wantErr bool
	}{
		{
			name:    "valid sha-256",
			ph:      addEventPayloadHash{Alg: "sha-256", Value: validValue},
			wantErr: false,
		},
		{
			name:    "alg case insensitive trimmed",
			ph:      addEventPayloadHash{Alg: "  sha-256  ", Value: validValue},
			wantErr: false,
		},
		{
			name:    "missing alg",
			ph:      addEventPayloadHash{Alg: "", Value: validValue},
			wantErr: true,
		},
		{
			name:    "wrong alg",
			ph:      addEventPayloadHash{Alg: "sha-1", Value: validValue},
			wantErr: true,
		},
		{
			name:    "invalid value",
			ph:      addEventPayloadHash{Alg: "sha-256", Value: "hex:tooshort"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePayloadHash(tc.ph)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRejectServerManagedFields(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "clean payload accepted",
			input:   `{"schema":"pa-notary-event@1","doc_uid":"DOC/1"}`,
			wantErr: false,
		},
		{
			name:    "event_id rejected",
			input:   `{"schema":"pa-notary-event@1","event_id":"some-id"}`,
			wantErr: true,
		},
		{
			name:    "recorded_at rejected",
			input:   `{"schema":"pa-notary-event@1","recorded_at":"2026-01-01T00:00:00Z"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON returns error",
			input:   `{not valid json`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectServerManagedFields([]byte(tc.input))
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
