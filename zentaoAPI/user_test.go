package zentaoapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestUserCtrlWire_ToUser_FullPayload(t *testing.T) {
	// Fixture mirrors the inner.user object returned by user-edit-1.json
	// GET on a real ZenTao Max 8.1 instance (probe-user-controller.md).
	// IDs come through as native ints here; the json.Number on the wire
	// type tolerates that AND the string-encoded form some endpoints use.
	raw := `{
		"id": 1,
		"company": "Sige",
		"type": "inside",
		"dept": 0,
		"account": "admin",
		"password": "e10adc3949ba59abbe56e057f20f883e",
		"role": "",
		"realname": "admin",
		"nickname": "",
		"commiter": "",
		"avatar": "",
		"birthday": "",
		"gender": "f",
		"email": "",
		"skype": "",
		"qq": "",
		"mobile": "",
		"phone": "",
		"weixin": "",
		"address": "",
		"visions": "rnd,lite",
		"visits": 636,
		"last": "2026-05-06 01:13:10",
		"fails": 0,
		"locked": "",
		"clientLang": "zh-cn",
		"deleted": "0"
	}`
	var w userCtrlWire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	u, err := w.toUser()
	if err != nil {
		t.Fatalf("toUser: %v", err)
	}
	if u.ID != 1 {
		t.Fatalf("ID = %d", u.ID)
	}
	if u.Account != "admin" {
		t.Fatalf("Account = %q", u.Account)
	}
	if u.Realname != "admin" {
		t.Fatalf("Realname = %q", u.Realname)
	}
	if u.Dept != 0 {
		t.Fatalf("Dept = %d", u.Dept)
	}
	if u.Type != "inside" {
		t.Fatalf("Type = %q", u.Type)
	}
	if u.Gender != "f" {
		t.Fatalf("Gender = %q", u.Gender)
	}
	if u.Visions != "rnd,lite" {
		t.Fatalf("Visions = %q", u.Visions)
	}
	if u.Visits != 636 {
		t.Fatalf("Visits = %d", u.Visits)
	}
	if u.Last != "2026-05-06 01:13:10" {
		t.Fatalf("Last = %q", u.Last)
	}
	if u.Deleted != "0" {
		t.Fatalf("Deleted = %q", u.Deleted)
	}
	if u.ClientLang != "zh-cn" {
		t.Fatalf("ClientLang = %q", u.ClientLang)
	}
	// Sensitive fields are NEVER populated from the wire — even if the
	// server echoes a stored hash under "password", User.Password stays
	// empty so callers can never accidentally log it round-tripped.
	if u.Password != "" || u.VerifyPassword != "" {
		t.Fatalf("password leaked into User: pw=%q verify=%q", u.Password, u.VerifyPassword)
	}
}

func TestUserCtrlWire_ToUser_StringEncodedNumbers(t *testing.T) {
	// Some Controller actions return numeric columns as JSON strings
	// rather than ints. json.Number on the wire absorbs both shapes.
	raw := `{
		"id": "42",
		"account": "alice",
		"realname": "Alice",
		"dept": "500",
		"visits": "0",
		"deleted": "0"
	}`
	var w userCtrlWire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	u, err := w.toUser()
	if err != nil {
		t.Fatalf("toUser: %v", err)
	}
	if u.ID != 42 {
		t.Fatalf("ID = %d", u.ID)
	}
	if u.Dept != 500 {
		t.Fatalf("Dept = %d", u.Dept)
	}
	if u.Visits != 0 {
		t.Fatalf("Visits = %d", u.Visits)
	}
}

func TestUserCtrlWire_ToUser_BadID(t *testing.T) {
	w := userCtrlWire{ID: json.Number("not-a-number")}
	if _, err := w.toUser(); err == nil {
		t.Fatal("expected error for malformed id")
	}
}

// --- CRUD method tests ---

func TestGetUser_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-edit-7.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// data is string-encoded JSON containing inner.user (per probe)
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"user\":{\"id\":7,\"account\":\"alice\",\"realname\":\"Alice\",\"dept\":3,\"role\":\"dev\",\"gender\":\"f\",\"visits\":12,\"deleted\":\"0\"}}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	u, err := c.GetUser(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.ID != 7 || u.Account != "alice" || u.Realname != "Alice" {
		t.Fatalf("user = %+v", u)
	}
	if u.Dept != 3 || u.Role != "dev" || u.Gender != "f" || u.Visits != 12 {
		t.Fatalf("user fields = %+v", u)
	}
}

func TestGetUser_NotFound_EmptyUserField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Missing/false user → no row.
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"user\":false}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetUser(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetUser_EnvelopeFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"User not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetUser(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (via classifyCtrlError)", err)
	}
}

func TestCreateUser_Happy(t *testing.T) {
	var gotMethod, gotPath, gotCT string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","load":"/zentao/company-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	u := &User{
		Account:  "alice",
		Password: "P@ssw0rd",
		Realname: "Alice",
		Dept:     500,
		Gender:   "f",
		Email:    "alice@example.test",
	}
	got, err := c.CreateUser(context.Background(), u)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/user-create.json" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type = %q", gotCT)
	}
	for k, want := range map[string]string{
		"account":  "alice",
		"password": "P@ssw0rd",
		"realname": "Alice",
		"dept":     "500",
		"gender":   "f",
		"email":    "alice@example.test",
		"visions":  "rnd", // default applied
	} {
		if gotForm.Get(k) != want {
			t.Errorf("form[%q] = %q, want %q", k, gotForm.Get(k), want)
		}
	}
	if got == nil {
		t.Fatal("expected non-nil user back")
	}
}

func TestCreateUser_LicenseCap_FlatMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":"系统用户人数已达授权的上限，不能继续添加用户！","load":"/zentao/company-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	u := &User{Account: "x", Password: "y", Realname: "z"}
	_, err := c.CreateUser(context.Background(), u)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if !strings.Contains(apiErr.Reason, "授权的上限") {
		t.Fatalf("reason = %q, want license message", apiErr.Reason)
	}
}

func TestCreateUser_PreflightRejection(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	cases := []struct {
		name string
		u    *User
	}{
		{"nil user", nil},
		{"empty account", &User{Password: "x", Realname: "y"}},
		{"empty password", &User{Account: "a", Realname: "y"}},
		{"empty realname", &User{Account: "a", Password: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.CreateUser(context.Background(), tc.u); err == nil {
				t.Fatal("expected pre-flight error")
			}
		})
	}
}

func TestUpdateUser_Happy_RefetchesAfterSuccess(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch hits {
		case 1:
			// edit POST
			if r.Method != http.MethodPost {
				t.Errorf("first request method = %s, want POST", r.Method)
			}
			_, _ = w.Write([]byte(`{"result":"success"}`))
		case 2:
			// re-fetch via edit GET
			if r.Method != http.MethodGet {
				t.Errorf("second request method = %s, want GET", r.Method)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":"{\"user\":{\"id\":7,\"account\":\"alice\",\"realname\":\"Alice Renamed\",\"deleted\":\"0\"}}"}`))
		default:
			t.Errorf("unexpected hit %d", hits)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	u := &User{ID: 7, Account: "alice", Realname: "Alice Renamed", VerifyPassword: "should-be-passed"}
	got, err := c.UpdateUser(context.Background(), u)
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if got.Realname != "Alice Renamed" {
		t.Fatalf("post-update realname = %q", got.Realname)
	}
	if hits != 2 {
		t.Fatalf("expected 2 server hits (edit + refetch), got %d", hits)
	}
}

func TestUpdateUser_VerifyPasswordFail_MapMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"verifyPassword":["验证失败，请检查您的系统登录密码是否正确"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.UpdateUser(context.Background(), &User{ID: 7, Account: "alice"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if !strings.Contains(apiErr.Reason, "verifyPassword") {
		t.Fatalf("reason should mention verifyPassword field, got %q", apiErr.Reason)
	}
}

func TestUpdateUser_PreflightRejection_NoID(t *testing.T) {
	c := newTestClient(t, "tok-1", "http://example.invalid")
	if _, err := c.UpdateUser(context.Background(), &User{Account: "a"}); err == nil {
		t.Fatal("expected pre-flight error for ID=0")
	}
}

func TestDeleteUser_Happy_ShapeA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("confirm") != "yes" {
			t.Errorf("missing confirm=yes")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":"{\"user\":false}"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteUser(context.Background(), 7); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func TestDeleteUser_Happy_ShapeC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","load":"/zentao/company-browse.json"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteUser(context.Background(), 7); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func TestDeleteUser_HTTP404_Idempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	if err := c.DeleteUser(context.Background(), 999); err != nil {
		t.Fatalf("DeleteUser on missing should be idempotent: %v", err)
	}
}

func TestUserToForm_AllOptionalFields(t *testing.T) {
	u := &User{
		Account: "alice", Password: "p", Realname: "Alice",
		VerifyPassword: "vp", Email: "a@b", Phone: "1", Mobile: "2", Dept: 9,
		Role: "dev", Gender: "f", Type: "inside", Nickname: "Al", Skype: "s",
		QQ: "q", Weixin: "w", Address: "addr", Birthday: "1990-01-01",
		Commiter: "c", Visions: "rnd,lite",
	}
	form := userToForm(u)
	for k, want := range map[string]string{
		"account": "alice", "password": "p", "realname": "Alice", "verifyPassword": "vp",
		"email": "a@b", "phone": "1", "mobile": "2", "dept": "9", "role": "dev",
		"gender": "f", "type": "inside", "nickname": "Al", "skype": "s", "qq": "q",
		"weixin": "w", "address": "addr", "birthday": "1990-01-01", "commiter": "c",
		"visions": "rnd,lite",
	} {
		if got := form.Get(k); got != want {
			t.Errorf("form[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestGetUser_MalformedEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	_, err := c.GetUser(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "decode get-user envelope") {
		t.Fatalf("err = %v, want envelope decode error", err)
	}
}

func TestDeleteUser_MalformedBodyFallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"unknown":"shape"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteUser(context.Background(), 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError fallback", err)
	}
}

func TestDeleteUser_VerifyPasswordFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"fail","message":{"verifyPassword":["验证失败"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok-1", srv.URL)
	err := c.DeleteUser(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
}
