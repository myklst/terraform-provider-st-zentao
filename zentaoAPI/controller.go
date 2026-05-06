package zentaoapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	backoff "github.com/cenkalti/backoff/v4"
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

// doControllerForm is a write-side counterpart to doController for
// Controller endpoints that ignore JSON bodies and require
// `application/x-www-form-urlencoded` instead — observed for ZenTao
// Max 8.x's user controller, where `user-edit` POST silently
// re-renders the GET form when handed JSON. Auth, refresh, retry and
// cookies follow the same pipeline as send(), but session-expiry
// replay is implemented inline because doRequest's send takes a JSON
// `body any`, not a pre-encoded form.
func (c *Client) doControllerForm(ctx context.Context, module, method string,
	pathArgs []string, query map[string]string, form url.Values,
) ([]byte, int, error) {
	path := controllerPath(module, method, pathArgs)
	encoded := form.Encode()

	c.tokenMu.Lock()
	observedToken := c.token
	c.tokenMu.Unlock()

	rawBody, status, location, err := c.sendForm(ctx, path, query, encoded, observedToken)
	if err != nil {
		return rawBody, status, err
	}
	if !isSessionExpired(status, rawBody, location) {
		return rawBody, status, nil
	}

	if err := c.refreshSession(ctx, observedToken); err != nil {
		return nil, status, fmt.Errorf("session refresh: %w", err)
	}
	c.tokenMu.Lock()
	newToken := c.token
	c.tokenMu.Unlock()

	rawBody, status, location, err = c.sendForm(ctx, path, query, encoded, newToken)
	if err != nil {
		return rawBody, status, err
	}
	if isSessionExpired(status, rawBody, location) {
		return rawBody, status, fmt.Errorf("session refresh exhausted: still expired after replay")
	}
	return rawBody, status, nil
}

// sendForm is doRequest's form-encoded twin — it shares URL composition,
// query-string handling (including the Controller-only zentaosid auto-
// inject), cookie jar, redirect suppression and 5xx backoff logic, but
// emits the body verbatim with `Content-Type: application/x-www-form-
// urlencoded`. This is duplicate code with send() by design; merging
// would require pushing body-encoding logic out of send (a wider
// refactor) and the duplication is small + isolated.
func (c *Client) sendForm(ctx context.Context, path string, query map[string]string, encoded, token string) ([]byte, int, string, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	q := endpoint.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	if token != "" && q.Get("zentaosid") == "" && !isV2Path(path) {
		q.Set("zentaosid", token)
	}
	endpoint.RawQuery = q.Encode()

	var (
		rawBody  []byte
		status   int
		location string
	)
	op := func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader([]byte(encoded)))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("build request: %w", err))
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		if token != "" {
			req.Header.Set("Token", token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		b, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		rawBody = b
		status = resp.StatusCode
		location = resp.Header.Get("Location")
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error %d", resp.StatusCode)
		}
		return nil
	}
	expo := backoff.NewExponentialBackOff()
	expo.InitialInterval = c.backoffInitialInterval
	expo.MaxElapsedTime = c.backoffMaxElapsed
	if err := backoff.Retry(op, backoff.WithContext(expo, ctx)); err != nil {
		return rawBody, status, location, err
	}
	return rawBody, status, location, nil
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
