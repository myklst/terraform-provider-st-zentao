package zentaoapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
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
	return zentaoFailReason(e.Error, e.Message, e.Reason)
}

// CtrlSimpleResponse is the envelope ZenTao Controllers return for
// write/form-submit operations (e.g. user-create, user-edit POST). It
// uses `result` (not `status`) and the `message` field can be either
// a flat string ("operation succeeded" / "license cap reached") OR a
// per-field validation map (`{fieldName: [errMsg1, ...]}`). Use
// IsSuccess for status, FieldErrors to disambiguate the message shape.
type CtrlSimpleResponse struct {
	Result  string          `json:"result"`
	Message json.RawMessage `json:"message,omitempty"`
	Load    string          `json:"load,omitempty"`
}

// IsSuccess reports whether the controller operation succeeded.
// ZenTao only ever uses "success" — anything else (including empty)
// means failure or unset.
func (r CtrlSimpleResponse) IsSuccess() bool {
	return r.Result == "success"
}

// FieldErrors disambiguates the dual-shape `message` field. For a flat
// string message it returns (msg, nil); for a per-field map it returns
// ("", map). When `message` is absent or unparseable as either shape,
// both return values are zero — caller can decide whether to treat as
// silent success or surface raw.
func (r CtrlSimpleResponse) FieldErrors() (string, map[string][]string) {
	if len(r.Message) == 0 || string(r.Message) == "null" {
		return "", nil
	}
	switch r.Message[0] {
	case '"':
		var s string
		if err := json.Unmarshal(r.Message, &s); err == nil {
			return s, nil
		}
	case '{':
		var m map[string][]string
		if err := json.Unmarshal(r.Message, &m); err == nil {
			return "", m
		}
	}
	return "", nil
}

// CtrlEnvelope is the envelope returned by ZenTao Controller endpoints
// (PATH_INFO `.json`). The resource payload sits inside Data, sometimes
// as a JSON-encoded string, sometimes as a direct object/array. Use
// DecodeData to unwrap it.
type CtrlEnvelope struct {
	Status  string          `json:"status"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	MD5     string          `json:"md5,omitempty"`
}

// ZentaoFailReason mirrors ZentaoResponse.ZentaoFailReason for the
// Controller envelope shape. Both delegate to the same helper.
func (e CtrlEnvelope) ZentaoFailReason() string {
	return zentaoFailReason(e.Error, e.Message, e.Reason)
}

// zentaoFailReason picks the most specific non-empty failure description
// across the three keys ZenTao uses interchangeably (error / message /
// reason). Shared by ZentaoResponse and CtrlEnvelope.
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

// isNotFoundReason recognises ZenTao's various ways of saying "the row
// you asked about does not exist" inside a 200-OK envelope. Used by
// both V2 wrappers and Controller error classification.
func isNotFoundReason(reason string) bool {
	r := strings.ToLower(reason)
	return strings.Contains(r, "not exist") || strings.Contains(r, "not found")
}

// isLoginRedirectReason recognises the body-level "session expired,
// please log in" signal. ZenTao Controllers sometimes return HTTP 200
// with this reason instead of a 302 redirect.
func isLoginRedirectReason(reason string) bool {
	if reason == "" {
		return false
	}
	r := strings.ToLower(reason)
	if strings.Contains(r, "please login") ||
		strings.Contains(r, "session expired") {
		return true
	}
	// Common Chinese localisations seen in zh-cn ZenTao deployments.
	if strings.Contains(reason, "请重新登录") ||
		strings.Contains(reason, "请登录") {
		return true
	}
	return false
}

// isUnauthorizedReason recognises a clear "credentials are wrong"
// signal. Other auth-related failures (malformed envelope, network
// glitch during login) should NOT be classified as ErrUnauthorized so
// callers can rely on errors.Is(err, ErrUnauthorized) meaning "the
// account/password really is no longer accepted".
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

// DecodeData unwraps env.Data into target. ZenTao Controllers return
// Data either as a JSON-encoded string (legacy v1 form: data is a
// quoted string whose contents are themselves JSON), or as a direct
// object/array (newer endpoints). Empty/null Data is a no-op success.
func DecodeData(env CtrlEnvelope, target any) error {
	data := env.Data
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	switch data[0] {
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("decode data string-wrapper: %w", err)
		}
		if s == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(s), target); err != nil {
			return fmt.Errorf("decode data inner JSON: %w", err)
		}
		return nil
	case '{', '[':
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("decode data: unexpected JSON shape %q", string(data))
	}
}

// classifyCtrlError maps a Controller failure response onto the
// appropriate sentinel or *APIError. Callers should only invoke it
// after confirming env.Status != "success"; session-expired cases
// (please login / 302→login) must be handled by the transport layer
// before this point.
func classifyCtrlError(httpStatus int, env CtrlEnvelope, rawBody []byte) error {
	reason := env.ZentaoFailReason()
	switch {
	case isNotFoundReason(reason):
		return ErrNotFound
	case isUnauthorizedReason(reason):
		return ErrUnauthorized
	default:
		return &APIError{
			HTTPStatus:   httpStatus,
			ZentaoStatus: env.Status,
			Reason:       reason,
			RawBody:      rawBody,
		}
	}
}
