// TODO: impostare una dimensione massima per il body (es. 3KB)
package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"merkle-log/server/internal/hashutil"

	jsoncanonicalizer "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
)

const eventSchema = "pa-notary-event@1"

type addEventRequest struct {
	Schema        string       `json:"schema"`
	EventType     string       `json:"event_type"`
	DocUID        string       `json:"doc_uid"`
	PrevEventID   *string      `json:"prev_event_id,omitempty"`
	PrevEventLeaf *int64       `json:"prev_event_leaf,omitempty"`
	PayloadHash   *PayloadHash `json:"payload_hash,omitempty"`
	Issuer        Issuer       `json:"issuer"`
	IssuedAt      string       `json:"issued_at"`
	Title         string       `json:"title"`
	Description   string       `json:"description,omitempty"`
	Notes         string       `json:"notes,omitempty"`
}

type PreparedEvent struct {
	Schema        string       `json:"schema"`
	EventID       string       `json:"event_id"`
	EventType     string       `json:"event_type"`
	DocUID        string       `json:"doc_uid"`
	DocVersion    int          `json:"doc_version"`
	PrevEventID   *string      `json:"prev_event_id,omitempty"`
	PrevEventLeaf *int64       `json:"prev_event_leaf,omitempty"`
	PayloadHash   *PayloadHash `json:"payload_hash,omitempty"`
	Issuer        Issuer       `json:"issuer"`
	IssuedAt      string       `json:"issued_at"`
	RecordedAt    string       `json:"recorded_at"`
	Title         string       `json:"title"`
	Description   string       `json:"description,omitempty"`
	Notes         string       `json:"notes,omitempty"`
}

type PayloadHash struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

type Issuer struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name,omitempty"`
}

func (e PreparedEvent) DocHash() (string, error) {
	if e.PayloadHash == nil {
		return "", nil
	}
	return hashutil.ParsePayloadHashValue(e.PayloadHash.Value)
}

func PrepareAddEventForNotarization(raw []byte, now time.Time, docVersion int) (PreparedEvent, []byte, error) {
	return PrepareAddEventForNotarizationWithMode(raw, now, docVersion, false)
}

func PrepareAddEventForNotarizationWithMode(raw []byte, now time.Time, docVersion int, useIssuedAtAsRecordedAt bool) (PreparedEvent, []byte, error) {
	if len(raw) == 0 {
		return PreparedEvent{}, nil, fmt.Errorf("empty body")
	}
	if docVersion < 1 {
		return PreparedEvent{}, nil, fmt.Errorf("doc_version must be >= 1")
	}

	req, err := decodeAddEventRequest(raw)
	if err != nil {
		return PreparedEvent{}, nil, err
	}

	docHash, issuedAt, err := validateAddEventRequest(req)
	if err != nil {
		return PreparedEvent{}, nil, err
	}

	recordedAt, err := computeRecordedAt(now, issuedAt, useIssuedAtAsRecordedAt)
	if err != nil {
		return PreparedEvent{}, nil, err
	}

	eventID := uuid.NewString()
	prepared := buildPreparedEvent(req, docVersion, docHash, issuedAt, recordedAt, eventID)

	canon, err := canonicalizePreparedEvent(prepared)
	if err != nil {
		return PreparedEvent{}, nil, err
	}

	return prepared, canon, nil
}

func decodeAddEventRequest(raw []byte) (addEventRequest, error) {
	if err := rejectServerManagedFields(raw); err != nil {
		return addEventRequest{}, err
	}

	var req addEventRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return addEventRequest{}, fmt.Errorf("invalid JSON structure: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return addEventRequest{}, fmt.Errorf("invalid trailing data in JSON body")
	}
	return req, nil
}

func computeRecordedAt(now, issuedAt time.Time, useIssuedAtAsRecordedAt bool) (time.Time, error) {
	recordedAt := now.UTC()
	if useIssuedAtAsRecordedAt {
		return issuedAt.UTC(), nil
	}
	if recordedAt.Before(issuedAt) { // TODO: gli orologi potrebbero non essere sincronizzati, valutare se accettare un margine di tolleranza (es. 5 secondi)
		return time.Time{}, fmt.Errorf("issued_at cannot be in the future")
	}
	return recordedAt, nil
}

func buildPreparedEvent(req addEventRequest, docVersion int, docHash string, issuedAt, recordedAt time.Time, eventID string) PreparedEvent {
	prevEventID := trimmedOptionalString(req.PrevEventID)
	canonicalPayloadHash := canonicalPayloadHash(req.PayloadHash != nil, docHash)

	return PreparedEvent{
		Schema:        req.Schema,
		EventID:       eventID,
		EventType:     normalizedEventType(req.EventType),
		DocUID:        strings.TrimSpace(req.DocUID),
		DocVersion:    docVersion,
		PrevEventID:   prevEventID,
		PrevEventLeaf: req.PrevEventLeaf,
		PayloadHash:   canonicalPayloadHash,
		Issuer: Issuer{
			EntityID: strings.TrimSpace(req.Issuer.EntityID),
			Name:     strings.TrimSpace(req.Issuer.Name),
		},
		IssuedAt:    issuedAt.Format(time.RFC3339Nano),
		RecordedAt:  recordedAt.Format(time.RFC3339Nano),
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Notes:       strings.TrimSpace(req.Notes),
	}
}

func canonicalizePreparedEvent(prepared PreparedEvent) ([]byte, error) {
	preparedRaw, err := json.Marshal(prepared)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize event: %w", err)
	}

	canon, err := jsoncanonicalizer.Transform(preparedRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize event JSON: %w", err)
	}
	return canon, nil
}

func trimmedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func canonicalPayloadHash(hasPayloadHash bool, docHash string) *PayloadHash {
	if !hasPayloadHash {
		return nil
	}
	return &PayloadHash{
		Alg:   "sha-256",
		Value: "hex:" + docHash,
	}
}

func normalizedEventType(eventType string) string {
	return strings.ToUpper(strings.TrimSpace(eventType))
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
	if _, exists := m["doc_version"]; exists {
		return fmt.Errorf("doc_version is managed by server and must not be provided")
	}
	return nil
}

func validateAddEventRequest(req addEventRequest) (string, time.Time, error) {
	if req.Schema != eventSchema {
		return "", time.Time{}, fmt.Errorf("schema must be %q", eventSchema)
	}

	if strings.TrimSpace(req.DocUID) == "" {
		return "", time.Time{}, fmt.Errorf("doc_uid is required")
	}
	if strings.TrimSpace(req.Issuer.EntityID) == "" {
		return "", time.Time{}, fmt.Errorf("issuer.entity_id is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return "", time.Time{}, fmt.Errorf("title is required")
	}
	if req.PrevEventLeaf != nil && *req.PrevEventLeaf < 0 {
		return "", time.Time{}, fmt.Errorf("prev_event_leaf must be >= 0")
	}

	issuedAt, err := parseRFC3339orNano(req.IssuedAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issued_at invalid: %v", err)
	}

	et := normalizedEventType(req.EventType)
	switch et {
	case "CREATE":
		if req.PrevEventID != nil && strings.TrimSpace(*req.PrevEventID) != "" {
			return "", time.Time{}, fmt.Errorf("CREATE must not include prev_event_id")
		}
		if req.PayloadHash == nil {
			return "", time.Time{}, fmt.Errorf("payload_hash is required for CREATE")
		}
		if err := validatePayloadHash(*req.PayloadHash); err != nil {
			return "", time.Time{}, err
		}
		docHash, err := hashutil.ParsePayloadHashValue(req.PayloadHash.Value)
		if err != nil {
			return "", time.Time{}, err
		}
		return docHash, issuedAt.UTC(), nil

	case "UPDATE":
		if err := requireUUIDv4Ptr("prev_event_id", req.PrevEventID); err != nil {
			return "", time.Time{}, err
		}
		if req.PayloadHash == nil {
			return "", time.Time{}, fmt.Errorf("payload_hash is required for UPDATE")
		}
		if err := validatePayloadHash(*req.PayloadHash); err != nil {
			return "", time.Time{}, err
		}
		docHash, err := hashutil.ParsePayloadHashValue(req.PayloadHash.Value)
		if err != nil {
			return "", time.Time{}, err
		}
		return docHash, issuedAt.UTC(), nil

	case "REVOKE", "EXPIRE":
		if err := requireUUIDv4Ptr("prev_event_id", req.PrevEventID); err != nil {
			return "", time.Time{}, err
		}
		if req.PayloadHash != nil {
			if err := validatePayloadHash(*req.PayloadHash); err != nil {
				return "", time.Time{}, err
			}
			docHash, err := hashutil.ParsePayloadHashValue(req.PayloadHash.Value)
			if err != nil {
				return "", time.Time{}, err
			}
			return docHash, issuedAt.UTC(), nil
		}
		return "", issuedAt.UTC(), nil

	default:
		return "", time.Time{}, fmt.Errorf("event_type invalid (got %q)", req.EventType)
	}
}

func validatePayloadHash(ph PayloadHash) error {
	alg := strings.TrimSpace(ph.Alg)
	if alg == "" {
		return fmt.Errorf("payload_hash.alg is required")
	}
	if alg != "sha-256" {
		return fmt.Errorf("payload_hash.alg must be sha-256")
	}

	if _, err := hashutil.ParsePayloadHashValue(ph.Value); err != nil {
		return err
	}
	return nil
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
