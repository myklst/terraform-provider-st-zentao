package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// controllerPath builds a ZenTao Controller URL path in the
// PATH_INFO `.json` form proven by the auth probe:
//
//	<module>-<method>[-<arg1>[-<arg2>...]].json
//
// The separator `-` matches the server's `requestFix` setting (the
// only value observed in the wild). Trailing empty args are trimmed —
// they would render as a meaningless dangling `-` on the wire — but
// middle empty args are preserved so callers can skip optional
// positional parameters in the middle of a method's signature, which
// some ZenTao Controllers require (e.g. `product-all-noclosed--25-1`
// where the orderBy slot is intentionally blank).
func controllerPath(module, method string, args []string) string {
	end := len(args)
	for end > 0 && args[end-1] == "" {
		end--
	}
	parts := make([]string, 0, 2+end)
	parts = append(parts, module, method)
	parts = append(parts, args[:end]...)
	return strings.Join(parts, "-") + ".json"
}

// doController invokes a ZenTao Controller endpoint via PATH_INFO
// `.json` routing. The HTTP method is derived from `body`: a nil body
// means GET (read operations like view/all/browse); any non-nil body
// promotes the call to POST (write operations like create/edit/close
// /assignTo). This convention matches the server's actual GET/POST
// dispatch rules per the design.
//
// Controller authentication: per probe (probe-controller-auth.md), the
// PATH_INFO routes authenticate EXCLUSIVELY via the `?zentaosid=` query
// parameter on Max 8.x — cookie + Token header alone yields 302 →
// /user-login. sendHTTP injects zentaosid for us when injectZentaosid=true.
//
// Quirks like `?confirm=yes` for delete are NOT injected here; typed
// wrappers are responsible for those because they're route-level
// rather than transport-level concerns.
func (c *Client) doController(
	ctx context.Context,
	module, method string,
	pathArgs []string,
	query map[string]string,
	body any,
) ([]byte, int, error) {
	httpMethod := http.MethodGet
	if body != nil {
		httpMethod = http.MethodPost
	}
	path := controllerPath(module, method, pathArgs)

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
	}
	return c.doWithRefresh(ctx, isControllerSessionExpired, func(token string) ([]byte, int, string, error) {
		return c.sendHTTP(ctx, httpMethod, path, query, bodyBytes, "application/json", token, true)
	})
}

// doControllerForm is a write-side counterpart to doController for
// Controller endpoints that ignore JSON bodies and require
// `application/x-www-form-urlencoded` instead — observed for ZenTao
// Max 8.x's user controller, where `user-edit` POST silently
// re-renders the GET form when handed JSON. Auth, refresh, retry and
// cookies all go through the same shared sendHTTP + doWithRefresh
// pipeline as doController; only the encoding differs.
func (c *Client) doControllerForm(
	ctx context.Context,
	module, method string,
	pathArgs []string,
	query map[string]string,
	form url.Values,
) ([]byte, int, error) {
	path := controllerPath(module, method, pathArgs)
	encoded := []byte(form.Encode())
	return c.doWithRefresh(ctx, isControllerSessionExpired, func(token string) ([]byte, int, string, error) {
		return c.sendHTTP(ctx, http.MethodPost, path, query, encoded, "application/x-www-form-urlencoded", token, true)
	})
}

// isControllerSessionExpired detects a "session is gone" signal on
// Controller routes. PATH_INFO Controllers express this two ways:
//   - 302 / 301 / 303 redirect with Location containing "user-login"
//     (the classic redirect-to-login signal; CheckRedirect is suppressed
//     in NewClient so we observe it instead of silently following).
//   - HTTP 200 carrying a CtrlEnvelope with a "please login" /
//     "请重新登录" reason — some ZenTao versions emit this body-level
//     signal alongside or instead of the redirect.
//
// V2-flavoured 401 is intentionally NOT recognised here — Controllers
// don't return 401, and conflating the two would mis-classify legitimate
// 401-returning Controller endpoints (none observed, but the namespace
// is symbolic).
func isControllerSessionExpired(httpStatus int, body []byte, location string) bool {
	if httpStatus == http.StatusFound || httpStatus == http.StatusMovedPermanently || httpStatus == http.StatusSeeOther {
		if location != "" && strings.Contains(location, "user-login") {
			return true
		}
		return false
	}
	if httpStatus == http.StatusOK && len(body) > 0 {
		var env CtrlResp
		if err := json.Unmarshal(body, &env); err == nil {
			if env.Status != "" && env.Status != "success" && isLoginRedirectReason(env.ZentaoFailReason()) {
				return true
			}
		}
	}
	return false
}

// ============================================================================
// Controller wire envelopes + their helpers
// ============================================================================

// CtrlResp is the envelope returned by ZenTao Controller endpoints
// (PATH_INFO `.json`). The resource payload sits inside Data, sometimes
// as a JSON-encoded string, sometimes as a direct object/array. Use
// DecodeData to unwrap it.
type CtrlResp struct {
	Status  string          `json:"status"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	MD5     string          `json:"md5,omitempty"`
}

// ZentaoFailReason mirrors ZentaoResponse.ZentaoFailReason for the
// Controller envelope shape. Both delegate to zentaoFailReason in
// errors.go.
func (e CtrlResp) ZentaoFailReason() string {
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
	// Load is shape-variant across modules: user-create / group-delete
	// return a redirect URL string, while group-create / group-edit
	// return a bool. We don't consume the value, so RawMessage absorbs
	// every shape without forcing transports into a custom decoder.
	Load json.RawMessage `json:"load,omitempty"`
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

// DecodeData unwraps e.Data into target. ZenTao Controllers return
// Data either as a JSON-encoded string (legacy v1 form: data is a
// quoted string whose contents are themselves JSON), or as a direct
// object/array (newer endpoints). Empty/null Data is a no-op success.
func (e CtrlResp) DecodeData(target any) error {
	data := e.Data
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
func classifyCtrlError(httpStatus int, env CtrlResp, rawBody []byte) error {
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

// classifyCtrlSimple is the shape-C counterpart to classifyCtrlError.
// It maps a CtrlSimpleResponse failure (`result:fail` with either flat
// or per-field message) onto a sentinel or *APIError. For per-field
// validation maps, it composes a single human-readable reason of the
// form "field1: err; field2: err" so the caller's logs / error chain
// stay one-liner-friendly.
func classifyCtrlSimple(httpStatus int, r CtrlSimpleResponse, rawBody []byte) error {
	flat, fields := r.FieldErrors()
	reason := flat
	if reason == "" && len(fields) > 0 {
		// stable order — sort field names for deterministic output
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %s", k, strings.Join(fields[k], ", ")))
		}
		reason = strings.Join(parts, "; ")
	}
	switch {
	case isNotFoundReason(reason):
		return ErrNotFound
	case isUnauthorizedReason(reason):
		return ErrUnauthorized
	default:
		return &APIError{
			HTTPStatus:   httpStatus,
			ZentaoStatus: r.Result,
			Reason:       reason,
			RawBody:      rawBody,
		}
	}
}

// isLoginRedirectReason recognises the body-level "session expired,
// please log in" signal. ZenTao Controllers sometimes return HTTP 200
// with this reason instead of a 302 redirect — see
// isControllerSessionExpired above for the integration point.
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

// CallController is an EXPERIMENTAL escape hatch for invoking ZenTao
// Controller endpoints not yet covered by typed wrappers. Prefer
// typed methods (GetUser, CreateProject, …) when they exist; this
// surface may change without notice as typed wrappers come online.
//
// The returned (body, status, err) are surfaced verbatim — the caller
// is responsible for envelope decoding (see CtrlEnvelope, DecodeData)
// and error classification (see classifyCtrlError). Session refresh
// and network retries are handled internally; only transport-level
// errors come back as a non-nil err.
func (c *Client) CallController(
	ctx context.Context,
	module, method string,
	pathArgs []string,
	query map[string]string,
	body any,
) ([]byte, int, error) {
	return c.doController(ctx, module, method, pathArgs, query, body)
}
