package api

import (
	"encoding/json"
	"testing"
)

// decodeResponse unmarshals an API response body, transparently peeling the
// {data: ...} single-resource and {data:{items:..., total:...}} list envelopes
// introduced by W2 so existing tests can keep asserting on the inner
// resource/list shape. For error/status bodies (without a "data" key) it falls
// back to decoding the whole body. It returns the unmarshal error so it can be
// used both as a bare statement and with `if err :=`.
func decodeResponse(t *testing.T, body []byte, v any) error {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal(body, &env); err == nil {
		if data, ok := env["data"]; ok {
			if len(data) > 0 && data[0] == '[' {
				return json.Unmarshal(data, v)
			}
			var inner map[string]json.RawMessage
			if json.Unmarshal(data, &inner) == nil && inner["items"] != nil {
				// Paginated list envelope: {data:{items:..., total:...}}.
				// The target is the list wrapper struct, so unmarshal `data`.
				return json.Unmarshal(data, v)
			}
			return json.Unmarshal(data, v)
		}
	}
	return json.Unmarshal(body, v)
}
