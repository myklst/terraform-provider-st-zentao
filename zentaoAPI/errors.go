package zentaoapi

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// errors.go owns the cross-transport error model: sentinels every
// caller compares against (ErrNotFound, ErrUnauthorized), the generic
// *APIError surfaced for any other failure, and the reason classifiers
// shared by V1 / V2 / Controller transports. Transport-specific
// envelope types and their classification helpers live alongside their
// transport (see apiv2_transport.go for ZentaoResponse, controller_transport.go
// for CtrlEnvelope / CtrlSimpleResponse / DecodeData / classifyCtrl*).

var (
	ErrNotFound      = errors.New("zentao: resource not found")
	ErrUnauthorized  = errors.New("zentao: invalid credentials")
	// ErrCycleDetected is the shared sentinel for any sibling-relation
	// mutator that rejects a parent assignment forming a cycle (self-attach
	// or multi-level ancestry loop). Not program-specific.
	ErrCycleDetected = errors.New("zentao: parent cycle detected")
)

// APIError is the structured failure surfaced when a response neither
// hits a sentinel nor parses as the expected business shape. Callers
// inspect via errors.As(err, &apiErr).
type APIError struct {
	HTTPStatus   int
	ZentaoStatus string
	Reason       string
	RawBody      []byte
}

// passwordPattern redacts `"password":"…"` from RawBody before it
// surfaces in error messages — keeps secrets out of logs even when a
// caller sends a credential payload that the server bounces back.
var passwordPattern = regexp.MustCompile(`("password"\s*:\s*")[^"]*(")`)

func (e *APIError) Error() string {
	redacted := passwordPattern.ReplaceAll(e.RawBody, []byte(`${1}***${2}`))
	return fmt.Sprintf("zentao api error: http=%d status=%q reason=%q body=%s",
		e.HTTPStatus, e.ZentaoStatus, e.Reason, string(redacted))
}

// zentaoFailReason picks the most specific non-empty failure description
// across the three keys ZenTao uses interchangeably (error / message /
// reason). Shared by ZentaoResponse (V2) and CtrlEnvelope (Controller).
func zentaoFailReason(err, message, reason string) string {
	switch {
	case err != "":
		return err
	case message != "":
		return message
	default:
		return reason
	}
}

// isNotFoundReason recognises ZenTao's various ways of saying "the row
// you asked about does not exist" inside a 200-OK envelope. Used by
// V2 wrappers (product / program) and Controller error classifiers.
func isNotFoundReason(reason string) bool {
	r := strings.ToLower(reason)
	return strings.Contains(r, "not exist") || strings.Contains(r, "not found")
}

// isUnauthorizedReason recognises a clear "credentials are wrong"
// signal. Other auth-related failures (malformed envelope, network
// glitch during login) should NOT be classified as ErrUnauthorized so
// callers can rely on errors.Is(err, ErrUnauthorized) meaning "the
// account/password really is no longer accepted".
//
// Cross-transport: used by Login (auth.go) on V1 login error envelopes
// and by classifyCtrlSimple (controller_transport.go) on Controller
// validation responses — same set of reason fragments works on both
// because ZenTao's error vocabulary is shared across transports.
func isUnauthorizedReason(reason string) bool {
	if reason == "" {
		return false
	}
	r := strings.ToLower(reason)
	if strings.Contains(r, "wrong") ||
		strings.Contains(r, "incorrect") ||
		strings.Contains(r, "invalid") {
		return true
	}
	if strings.Contains(reason, "密码错误") ||
		strings.Contains(reason, "登录失败") ||
		strings.Contains(reason, "认证") {
		return true
	}
	return false
}
