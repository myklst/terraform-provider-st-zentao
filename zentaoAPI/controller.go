package zentaoapi

import (
	"context"
	"net/http"
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
// dispatch rules per the design (see grill-me Q6).
//
// Auth, session refresh, network retry and cookie persistence are
// inherited from doRequest — Controller transport reuses the same
// pipeline so a single Login covers both V2 and Controller routes.
//
// Quirks like `?confirm=yes` for delete are NOT injected here; typed
// wrappers are responsible for those because they're route-level
// rather than transport-level concerns.
func (c *Client) doController(ctx context.Context, module, method string,
	pathArgs []string, query map[string]string, body any,
) ([]byte, int, error) {
	httpMethod := http.MethodGet
	if body != nil {
		httpMethod = http.MethodPost
	}
	return c.doRequest(ctx, httpMethod, controllerPath(module, method, pathArgs), query, body)
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
func (c *Client) CallController(ctx context.Context, module, method string,
	pathArgs []string, query map[string]string, body any,
) ([]byte, int, error) {
	return c.doController(ctx, module, method, pathArgs, query, body)
}
