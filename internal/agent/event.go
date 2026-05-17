//go:build !lite

package agent

import "encoding/json"

// EventType constants for the sidecar's NDJSON stream.
const (
	EventTypeAssistantMessage = "assistant_message"
	EventTypeToolUse          = "tool_use"
	EventTypeToolResult       = "tool_result"
	EventTypeResult           = "result"
	EventTypeError            = "error"
	EventTypeSystem           = "system"
	EventTypeCostCapHit       = "cost_cap_hit"
)

// Event is one line of the sidecar's stdout NDJSON stream. Raw preserves the
// full JSON for forward compatibility; typed accessors below unmarshal it.
type Event struct {
	Type string          `json:"type"`
	TS   int64           `json:"ts"`
	Raw  json.RawMessage `json:"-"`
}

// ResultPayload is the payload when Type == EventTypeResult. Mirrors the
// fields the sidecar populates from the SDK's terminal ResultMessage.
type ResultPayload struct {
	TotalCostUSD        float64 `json:"totalCostUsd"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	TurnCount           int     `json:"turnCount"`
	NumToolCalls        int     `json:"numToolCalls"`
	SessionID           string  `json:"sessionId"`
	StopReason          string  `json:"stopReason"`
}

// ErrorPayload is the payload when Type == EventTypeError.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// envelope is used for raw-only unmarshal so we can grab Type/TS without
// committing to the full payload shape.
type envelope struct {
	Type string `json:"type"`
	TS   int64  `json:"ts"`
}

// ParseEvent unmarshals a single NDJSON line into an Event and stashes the
// raw bytes for downstream typed access. Returns an error only on malformed JSON.
func ParseEvent(line []byte) (Event, error) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Event{}, err
	}
	// Copy the bytes so the caller's buffer can be reused.
	raw := make(json.RawMessage, len(line))
	copy(raw, line)
	return Event{Type: env.Type, TS: env.TS, Raw: raw}, nil
}

// ParseResult unmarshals the raw JSON into a ResultPayload.
// Only meaningful when e.Type == EventTypeResult — but ResultPayload's
// fields live alongside the envelope, not nested under "data", so the
// raw bytes work as-is.
func (e Event) ParseResult() (ResultPayload, error) {
	// The sidecar emits {type, ts, data: {...payload...}}. Unwrap data.
	var wrapper struct {
		Data ResultPayload `json:"data"`
	}
	if err := json.Unmarshal(e.Raw, &wrapper); err != nil {
		return ResultPayload{}, err
	}
	return wrapper.Data, nil
}

// ParseError unmarshals the raw JSON into an ErrorPayload.
func (e Event) ParseError() (ErrorPayload, error) {
	var wrapper struct {
		Data ErrorPayload `json:"data"`
	}
	if err := json.Unmarshal(e.Raw, &wrapper); err != nil {
		return ErrorPayload{}, err
	}
	return wrapper.Data, nil
}
