//go:build integration

// Build tag `integration` keeps this off the default `go test` run so
// CI can stay hermetic. Invoke with:
//
//	go test -tags=integration -run TestIntegration_V1Login_AndCallController ./zentaoAPI/...
//
// And export ZENTAO_URL, ZENTAO_ACCOUNT,
// ZENTAO_PASSWORD against a real ZenTao instance you control.

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
// Skipped if ZENTAO_URL is unset.
func TestIntegration_V1Login_AndCallController(t *testing.T) {
	url := os.Getenv("ZENTAO_URL")
	account := os.Getenv("ZENTAO_ACCOUNT")
	password := os.Getenv("ZENTAO_PASSWORD")
	if url == "" || account == "" || password == "" {
		t.Skip("set ZENTAO_URL / _ACCOUNT / _PASSWORD to run")
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

	var env CtrlResp
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
	if err := env.DecodeData(&generic); err != nil {
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

// TestIntegration_GetUser_Admin verifies the user controller's read
// primitive against a real instance. We can only round-trip GetUser
// on this branch — CreateUser is blocked by the licensed-user cap and
// UpdateUser/DeleteUser are blocked by ZenTao Max 8.1's verifyPassword
// sudo gate (see docs/superpowers/specs/probe-user-controller.md).
//
// Skipped if ZENTAO_URL is unset.
func TestIntegration_GetUser_Admin(t *testing.T) {
	url := os.Getenv("ZENTAO_URL")
	account := os.Getenv("ZENTAO_ACCOUNT")
	password := os.Getenv("ZENTAO_PASSWORD")
	if url == "" || account == "" || password == "" {
		t.Skip("set ZENTAO_URL / _ACCOUNT / _PASSWORD to run")
	}

	c, err := NewClient(url, account, password)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// admin is conventionally id=1 on every ZenTao instance.
	u, err := c.GetUser(ctx, 1)
	if err != nil {
		t.Fatalf("GetUser(1): %v", err)
	}
	if u.ID != 1 {
		t.Fatalf("ID = %d, want 1", u.ID)
	}
	if u.Account != account {
		t.Fatalf("Account = %q, want %q", u.Account, account)
	}
	if u.Realname == "" {
		t.Fatal("Realname unexpectedly empty")
	}
	// Sensitive discipline: read-side must NEVER populate Password.
	if u.Password != "" {
		t.Fatalf("Password leaked into User on read: %q", u.Password)
	}
	if u.VerifyPassword != "" {
		t.Fatalf("VerifyPassword unexpectedly populated: %q", u.VerifyPassword)
	}
}
