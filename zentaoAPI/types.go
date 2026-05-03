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
//
// Failure descriptions arrive under three different keys depending on
// which v2 handler answered: "error" (most common), "message" (e.g.
// product DELETE on a missing row), and "reason" (legacy v1 carryover
// some endpoints still emit). All three are decoded so ZentaoFailReason
// can pick whichever one was populated.
type ZentaoResponse struct {
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// ZentaoFailReason returns whichever non-empty failure description the
// envelope carried, preferring the most specific shape observed in the
// wild: error → message → reason.
func (e ZentaoResponse) ZentaoFailReason() string {
	switch {
	case e.Error != "":
		return e.Error
	case e.Message != "":
		return e.Message
	default:
		return e.Reason
	}
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
