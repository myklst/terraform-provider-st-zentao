package zentaoapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

type ZentaoResponse struct {
	Status string          `json:"status"`
	Reason string          `json:"reason,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

var (
	ErrNotFound     = errors.New("zentao: resource not found")
	ErrUnauthorized = errors.New("zentao: invalid credentials")
)

type APIError struct {
	HTTPStatus   int
	ZentaoStatus string
	Reason       string
	RawBody      []byte
}

var passwordPattern = regexp.MustCompile(`("password"\s*:\s*")[^"]*(")`)

func (e *APIError) Error() string {
	redacted := passwordPattern.ReplaceAll(e.RawBody, []byte(`${1}***${2}`))
	return fmt.Sprintf("zentao api error: http=%d status=%q reason=%q body=%s",
		e.HTTPStatus, e.ZentaoStatus, e.Reason, string(redacted))
}

// decodeData unmarshals env.Data into target. ZenTao APIs sometimes return
// data as a JSON object directly, and sometimes as a JSON string containing
// JSON. This helper handles both shapes transparently.
func decodeData(env *ZentaoResponse, target any) error {
	raw := env.Data
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var inner string
		if err := json.Unmarshal(raw, &inner); err != nil {
			return fmt.Errorf("decode data string: %w", err)
		}
		if inner == "" {
			return nil
		}
		return json.Unmarshal([]byte(inner), target)
	}
	return json.Unmarshal(raw, target)
}
