package notaryapi

import "encoding/json"

type PayloadHash struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

type Issuer struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name,omitempty"`
}

type AddEventRequest struct {
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

type AddEventResponse struct {
	LogIndex     uint64          `json:"log_index"`
	NotarizedRaw json.RawMessage `json:"notarized_json"`
}
