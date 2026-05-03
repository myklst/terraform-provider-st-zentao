package zentaoapi

import (
	"errors"
	"fmt"
	"regexp"
)

// ZentaoResponse is the minimal envelope shared by all v2 endpoints.
// v2 responses are flat: {"status":"success", ...resource fields...} or
// {"status":"fail", "error":"..."} — no nested "data" wrapper, so the
// concrete fields are decoded by per-endpoint structs that embed Status.
type ZentaoResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ZentaoFailReason returns whichever non-empty failure description the
// envelope carried. v2 uses "error", v1 used "reason"; both are tolerated
// so callers don't care.
func (e ZentaoResponse) ZentaoFailReason() string {
	if e.Error != "" {
		return e.Error
	}
	return e.Reason
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
