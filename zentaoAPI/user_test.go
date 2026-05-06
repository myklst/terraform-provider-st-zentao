package zentaoapi

import (
	"encoding/json"
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
