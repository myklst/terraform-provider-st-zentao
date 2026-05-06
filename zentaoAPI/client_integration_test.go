//go:build integration

// Build tag `integration` keeps this off the default `go test` run so
// CI can stay hermetic. Invoke with:
//
//	go test -tags=integration -run TestIntegration_V1Login_AndCallController ./zentaoAPI/...
//
// And export ZENTAO_INTEGRATION_URL, ZENTAO_INTEGRATION_ACCOUNT,
// ZENTAO_INTEGRATION_PASSWORD against a real ZenTao instance you control.

package zentaoapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestIntegration_V1Login_AndCallController verifies the v1 two-step
// Login flow and the Controller transport against a real ZenTao
// instance. It is the regression gate for Phase B+C against drift in
// real server behaviour: if ZenTao changes the envelope shape, login
// route, or Controller dispatch, this test catches it.
//
// Skipped if ZENTAO_INTEGRATION_URL is unset.
func TestIntegration_V1Login_AndCallController(t *testing.T) {
	url := os.Getenv("ZENTAO_INTEGRATION_URL")
	account := os.Getenv("ZENTAO_INTEGRATION_ACCOUNT")
	password := os.Getenv("ZENTAO_INTEGRATION_PASSWORD")
	if url == "" || account == "" || password == "" {
		t.Skip("set ZENTAO_INTEGRATION_URL / _ACCOUNT / _PASSWORD to run")
	}

	c, err := NewClient(url, account, password)
	if err != nil {
		t.Fatalf("NewClient (Login): %v", err)
	}
	if c.token == "" {
		t.Fatal("Login produced empty token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	body, status, err := c.CallController(ctx, "product", "all", nil, nil, nil)
	if err != nil {
		t.Fatalf("CallController(product/all): %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d (body=%s)", status, body)
	}

	var env CtrlEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("envelope decode: %v (body=%s)", err, body)
	}
	if env.Status != "success" {
		t.Fatalf("envelope status = %q, body=%s", env.Status, body)
	}

	// product-all should return at least metadata under data; we don't
	// assert on exact contents (instance-specific) but we do require
	// DecodeData succeeds on the shape ZenTao actually returns.
	var generic map[string]any
	if err := DecodeData(env, &generic); err != nil {
		t.Fatalf("DecodeData: %v (data=%s)", err, env.Data)
	}
	if len(generic) == 0 {
		t.Logf("product-all data was empty — instance has no products")
	}

	// Sanity: a second call uses the established cookie+token. If the
	// jar lost cookies between requests, ZenTao would 302 to login on
	// the Controller side; our refresh would then fire a second Login
	// (which would also work but indicates a regression).
	_, status2, err2 := c.CallController(ctx, "product", "all", nil, nil, nil)
	if err2 != nil {
		t.Fatalf("second CallController: %v", err2)
	}
	if status2 != 200 {
		t.Fatalf("second call status = %d", status2)
	}
	_ = strings.TrimSpace // keep imports stable if test grows
}
