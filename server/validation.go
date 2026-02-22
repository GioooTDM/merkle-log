// TODO: impostare una dimensione massima per il body (es. 3KB)
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	jsoncanonicalizer "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
)

const eventSchema = "pa-notary-event@1"

type validatedAddEvent struct {
	DocUID  string
	EventID string
	DocHash string
}

type addEventClientPayload struct {
	Schema        string               `json:"schema"`
	EventType     string               `json:"event_type"`
	DocUID        string               `json:"doc_uid"`
	DocVersion    int                  `json:"doc_version"`
	PrevEventID   *string              `json:"prev_event_id,omitempty"`
	PrevEventLeaf *int64               `json:"prev_event_leaf,omitempty"`
	PayloadHash   *addEventPayloadHash `json:"payload_hash,omitempty"`
	Issuer        addEventIssuer       `json:"issuer"`
	IssuedAt      string               `json:"issued_at"`
	Title         string               `json:"title"`
	Description   string               `json:"description,omitempty"`
	Notes         string               `json:"notes,omitempty"`
}

type addEventPayload struct {
	Schema        string               `json:"schema"`
	EventID       string               `json:"event_id"`
	EventType     string               `json:"event_type"`
	DocUID        string               `json:"doc_uid"`
	DocVersion    int                  `json:"doc_version"`
	PrevEventID   *string              `json:"prev_event_id,omitempty"`
	PrevEventLeaf *int64               `json:"prev_event_leaf,omitempty"`
	PayloadHash   *addEventPayloadHash `json:"payload_hash,omitempty"`
	Issuer        addEventIssuer       `json:"issuer"`
	IssuedAt      string               `json:"issued_at"`
	RecordedAt    string               `json:"recorded_at"`
	Title         string               `json:"title"`
	Description   string               `json:"description,omitempty"`
	Notes         string               `json:"notes,omitempty"`
}

type addEventPayloadHash struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

type addEventIssuer struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name,omitempty"`
}

func prepareAddEventForNotarization(raw []byte, now time.Time) (validatedAddEvent, []byte, error) {
	if len(raw) == 0 {
		return validatedAddEvent{}, nil, fmt.Errorf("empty body")
	}

	if err := rejectServerManagedFields(raw); err != nil {
		return validatedAddEvent{}, nil, err
	}

	var ev addEventClientPayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return validatedAddEvent{}, nil, fmt.Errorf("invalid JSON structure: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return validatedAddEvent{}, nil, fmt.Errorf("invalid trailing data in JSON body")
	}

	docHash, issuedAt, err := validateClientEventSemantics(ev)
	if err != nil {
		return validatedAddEvent{}, nil, err
	}

	// TODO: gli orologi potrebbero non essere sincronizzati, valutare se accettare un margine di tolleranza (es. 5 secondi)
	recordedAt := now.UTC()
	if recordedAt.Before(issuedAt) {
		return validatedAddEvent{}, nil, fmt.Errorf("issued_at cannot be in the future")
	}

	eventID := uuid.NewString()
	var prevEventID *string
	if ev.PrevEventID != nil {
		trimmedPrev := strings.TrimSpace(*ev.PrevEventID)
		prevEventID = &trimmedPrev
	}
	var payloadHash *addEventPayloadHash
	if ev.PayloadHash != nil {
		payloadHash = &addEventPayloadHash{
			Alg:   "sha-256",
			Value: "hex:" + docHash,
		}
	}

	stored := addEventPayload{
		Schema:        ev.Schema,
		EventID:       eventID,
		EventType:     strings.ToUpper(strings.TrimSpace(ev.EventType)),
		DocUID:        strings.TrimSpace(ev.DocUID),
		DocVersion:    ev.DocVersion,
		PrevEventID:   prevEventID,
		PrevEventLeaf: ev.PrevEventLeaf,
		PayloadHash:   payloadHash,
		Issuer: addEventIssuer{
			EntityID: strings.TrimSpace(ev.Issuer.EntityID),
			Name:     strings.TrimSpace(ev.Issuer.Name),
		},
		IssuedAt:    issuedAt.Format(time.RFC3339Nano),
		RecordedAt:  recordedAt.Format(time.RFC3339Nano),
		Title:       strings.TrimSpace(ev.Title),
		Description: strings.TrimSpace(ev.Description),
		Notes:       strings.TrimSpace(ev.Notes),
	}

	storedRaw, err := json.Marshal(stored)
	if err != nil {
		return validatedAddEvent{}, nil, fmt.Errorf("failed to serialize event: %w", err)
	}

	canon, err := jsoncanonicalizer.Transform(storedRaw)
	if err != nil {
		return validatedAddEvent{}, nil, fmt.Errorf("failed to canonicalize event JSON: %w", err)
	}

	return validatedAddEvent{
		DocUID:  stored.DocUID,
		EventID: eventID,
		DocHash: docHash,
	}, canon, nil
}

func rejectServerManagedFields(raw []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, exists := m["event_id"]; exists {
		return fmt.Errorf("event_id is managed by server and must not be provided")
	}
	if _, exists := m["recorded_at"]; exists {
		return fmt.Errorf("recorded_at is managed by server and must not be provided")
	}
	return nil
}

func validateClientEventSemantics(ev addEventClientPayload) (string, time.Time, error) {
	if ev.Schema != eventSchema {
		return "", time.Time{}, fmt.Errorf("schema must be %q", eventSchema)
	}

	if strings.TrimSpace(ev.DocUID) == "" {
		return "", time.Time{}, fmt.Errorf("doc_uid is required")
	}
	if ev.DocVersion < 1 {
		return "", time.Time{}, fmt.Errorf("doc_version must be >= 1")
	}
	if strings.TrimSpace(ev.Issuer.EntityID) == "" {
		return "", time.Time{}, fmt.Errorf("issuer.entity_id is required")
	}
	if strings.TrimSpace(ev.Title) == "" {
		return "", time.Time{}, fmt.Errorf("title is required")
	}
	if ev.PrevEventLeaf != nil && *ev.PrevEventLeaf < 0 {
		return "", time.Time{}, fmt.Errorf("prev_event_leaf must be >= 0")
	}

	issuedAt, err := parseRFC3339orNano(ev.IssuedAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issued_at invalid: %v", err)
	}

	et := strings.ToUpper(strings.TrimSpace(ev.EventType))
	switch et {
	case "CREATE":
		if ev.DocVersion != 1 {
			return "", time.Time{}, fmt.Errorf("CREATE requires doc_version=1")
		}
		if ev.PrevEventID != nil && strings.TrimSpace(*ev.PrevEventID) != "" {
			return "", time.Time{}, fmt.Errorf("CREATE must not include prev_event_id")
		}
		if ev.PayloadHash == nil {
			return "", time.Time{}, fmt.Errorf("payload_hash is required for CREATE")
		}
		if err := validatePayloadHash(*ev.PayloadHash); err != nil {
			return "", time.Time{}, err
		}
		docHash, err := parsePayloadHashValue(ev.PayloadHash.Value)
		if err != nil {
			return "", time.Time{}, err
		}
		return docHash, issuedAt.UTC(), nil

	case "UPDATE":
		if ev.DocVersion < 2 {
			return "", time.Time{}, fmt.Errorf("UPDATE requires doc_version>=2")
		}
		if err := requireUUIDv4Ptr("prev_event_id", ev.PrevEventID); err != nil {
			return "", time.Time{}, err
		}
		if ev.PayloadHash == nil {
			return "", time.Time{}, fmt.Errorf("payload_hash is required for UPDATE")
		}
		if err := validatePayloadHash(*ev.PayloadHash); err != nil {
			return "", time.Time{}, err
		}
		docHash, err := parsePayloadHashValue(ev.PayloadHash.Value)
		if err != nil {
			return "", time.Time{}, err
		}
		return docHash, issuedAt.UTC(), nil

	case "REVOKE", "EXPIRE":
		if err := requireUUIDv4Ptr("prev_event_id", ev.PrevEventID); err != nil {
			return "", time.Time{}, err
		}
		if ev.PayloadHash != nil {
			if err := validatePayloadHash(*ev.PayloadHash); err != nil {
				return "", time.Time{}, err
			}
			docHash, err := parsePayloadHashValue(ev.PayloadHash.Value)
			if err != nil {
				return "", time.Time{}, err
			}
			return docHash, issuedAt.UTC(), nil
		}
		return "", issuedAt.UTC(), nil

	default:
		return "", time.Time{}, fmt.Errorf("event_type invalid (got %q)", ev.EventType)
	}
}

func validatePayloadHash(ph addEventPayloadHash) error {
	alg := strings.TrimSpace(ph.Alg)
	if alg == "" {
		return fmt.Errorf("payload_hash.alg is required")
	}
	if alg != "sha-256" {
		return fmt.Errorf("payload_hash.alg must be sha-256")
	}

	if _, err := parsePayloadHashValue(ph.Value); err != nil {
		return err
	}
	return nil
}

func parsePayloadHashValue(v string) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", fmt.Errorf("payload_hash.value is required")
	}

	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "hex:") {
		return "", fmt.Errorf(`payload_hash.value must use "hex:<digest>" format`)
	}
	hexPart := lower[len("hex:"):]
	if len(hexPart) != 64 {
		return "", fmt.Errorf("payload_hash.value must contain 64 hex chars for sha-256")
	}

	raw, err := hex.DecodeString(hexPart)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("payload_hash.value is not valid sha-256 hex")
	}
	return hexPart, nil
}

func requireUUIDv4(field, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("%s is required", field)
	}

	id, err := uuid.Parse(v)
	if err != nil {
		return fmt.Errorf("%s invalid UUID", field)
	}
	if id.Version() != 4 {
		return fmt.Errorf("%s must be UUIDv4", field)
	}
	return nil
}

func requireUUIDv4Ptr(field string, value *string) error {
	if value == nil {
		return fmt.Errorf("%s is required", field)
	}
	return requireUUIDv4(field, *value)
}

func parseRFC3339orNano(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
