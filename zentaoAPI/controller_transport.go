package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
		var env CtrlEnvelope
		if err := json.Unmarshal(body, &env); err == nil {
			if env.Status != "" && env.Status != "success" && isLoginRedirectReason(env.ZentaoFailReason()) {
				return true
			}
		}
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
