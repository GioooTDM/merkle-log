package event

import (
	"encoding/json"
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
		ph      PayloadHash
		wantErr bool
	}{
		{
			name:    "valid sha-256",
			ph:      PayloadHash{Alg: "sha-256", Value: validValue},
			wantErr: false,
		},
		{
			name:    "alg case insensitive trimmed",
			ph:      PayloadHash{Alg: "  sha-256  ", Value: validValue},
			wantErr: false,
		},
		{
			name:    "missing alg",
			ph:      PayloadHash{Alg: "", Value: validValue},
			wantErr: true,
		},
		{
			name:    "wrong alg",
			ph:      PayloadHash{Alg: "sha-1", Value: validValue},
			wantErr: true,
		},
		{
			name:    "invalid value",
			ph:      PayloadHash{Alg: "sha-256", Value: "hex:tooshort"},
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
			name:    "doc_version rejected",
			input:   `{"schema":"pa-notary-event@1","doc_version":2}`,
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

func TestDecodeAddEventRequest(t *testing.T) {
	validHash := "hex:aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb"

	tests := []struct {
		name    string
		input   []byte
		wantErr bool
		check   func(t *testing.T, got AddEventRequest)
	}{
		{
			name:    "empty body",
			input:   []byte{},
			wantErr: true,
		},
		{
			name: "fields are normalized",
			input: mustJSON(t, map[string]any{
				"schema":        "pa-notary-event@1",
				"event_type":    " update ",
				"doc_uid":       " DOC/1 ",
				"prev_event_id": " 550e8400-e29b-41d4-a716-446655440000 ",
				"payload_hash": map[string]any{
					"alg":   "sha-256",
					"value": validHash,
				},
				"issuer": map[string]any{
					"entity_id": "  IPA:TEST  ",
					"name":      "  Ente Test  ",
				},
				"issued_at":   "2026-01-01T10:00:00Z",
				"title":       "  Titolo  ",
				"description": "  Desc  ",
				"notes":       "  Note  ",
			}),
			check: func(t *testing.T, got AddEventRequest) {
				t.Helper()
				if got.EventType != "UPDATE" {
					t.Errorf("EventType = %q, want UPDATE", got.EventType)
				}
				if got.DocUID != "DOC/1" {
					t.Errorf("DocUID = %q, want DOC/1", got.DocUID)
				}
				if got.PrevEventID == nil || *got.PrevEventID != "550e8400-e29b-41d4-a716-446655440000" {
					t.Errorf("PrevEventID = %v, want trimmed UUID", got.PrevEventID)
				}
				if got.Issuer.EntityID != "IPA:TEST" {
					t.Errorf("Issuer.EntityID = %q, want IPA:TEST", got.Issuer.EntityID)
				}
				if got.Issuer.Name != "Ente Test" {
					t.Errorf("Issuer.Name = %q, want Ente Test", got.Issuer.Name)
				}
				if got.Title != "Titolo" {
					t.Errorf("Title = %q, want Titolo", got.Title)
				}
				if got.Description != "Desc" {
					t.Errorf("Description = %q, want Desc", got.Description)
				}
				if got.Notes != "Note" {
					t.Errorf("Notes = %q, want Note", got.Notes)
				}
			},
		},
		{
			name: "server managed fields rejected",
			input: mustJSON(t, map[string]any{
				"schema":   "pa-notary-event@1",
				"event_id": "some-id",
			}),
			wantErr: true,
		},
		{
			name:    "invalid JSON rejected",
			input:   []byte(`{not valid`),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeAddEventRequest(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
