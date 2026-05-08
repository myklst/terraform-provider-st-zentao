package zentaoapi

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

// client_test.go covers *Client construction (NewClient) and shared
// test helpers used across every transport test file. Per-transport
// behaviour lives in:
//   - apiv1_transport_test.go (V1 transport: doV1Request, isV1SessionExpired)
//   - apiv2_transport_test.go (V2 transport: doV2Request, isV2SessionExpired,
//     plus shared sendHTTP smoke-tests exercised through V2)
//   - controller_transport_test.go (Controller transport: doController,
//     doControllerForm, isControllerSessionExpired, CallController)
// Login + refreshSession unit tests live in auth_test.go.

// --- NewClient construction sanity ---

func TestNewClient_HasCookieJar(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{})
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.http.Jar == nil {
		t.Fatal("expected http.Client.Jar to be initialised")
	}
}

func TestNewClient_CheckRedirectDisabled(t *testing.T) {
	srv := newV1LoginServer(t, v1Opts{})
	defer srv.Close()

	c, err := NewClient(srv.URL, "admin", "p")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.http.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be set (block auto-follow)")
	}
	got := c.http.CheckRedirect(nil, nil)
	if got != http.ErrUseLastResponse {
		t.Fatalf("CheckRedirect = %v, want http.ErrUseLastResponse", got)
	}
}

// --- shared test helpers (consumed by every *_transport_test.go) ---

func newTestClient(t *testing.T, token string, srvURL string) *Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &Client{
		baseURL:  mustParseURL(t, srvURL),
		account:  "admin",
		password: "p",
		token:    token,
		http: &http.Client{
			Jar: jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		backoffMaxElapsed:      2 * time.Second,
		backoffInitialInterval: 1 * time.Millisecond,
	}
	return c
}

func mustParseURL(t *testing.T, raw string) *jsonURL {
	t.Helper()
	u, err := parseBaseURL(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

// readAllString drains an io.Reader for ergonomic test assertions.
func readAllString(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}

// silence unused-import warning when readAllString is unused.
var _ = readAllString
