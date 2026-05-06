package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// User is the canonical in-memory representation of a ZenTao user.
//
// Read sites populate the fields tagged for the wire (via userCtrlWire);
// the two sensitive fields (Password / VerifyPassword) are write-only —
// the server's stored password hash is intentionally NEVER round-tripped
// onto User so it can't leak through logging or error formatting.
//
// VerifyPassword is the field some ZenTao instances require for sudo
// confirmation on edit/delete operations against the user controller
// (observed: ZenTao Max 8.1). Callers fill it when their instance
// enforces the gate; the wrapper does not attempt to compute or
// pre-hash it because the gate's hashing scheme isn't documented and
// varies across editions.
type User struct {
	// Identity & immutable.
	ID      int    `json:"-"` // assigned by server; not sent on create.
	Account string `json:"account"`

	// Writeable & commonly used fields. Optional fields use omitempty
	// so partial updates don't blank server-side defaults.
	Realname string `json:"realname"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Mobile   string `json:"mobile,omitempty"`
	Dept     int    `json:"dept,omitempty"`
	Role     string `json:"role,omitempty"`
	Gender   string `json:"gender,omitempty"`  // "m" | "f"
	Visions  string `json:"visions,omitempty"` // "rnd" | "lite" | "rnd,lite"
	Type     string `json:"type,omitempty"`    // "inside" | "outside"
	Nickname string `json:"nickname,omitempty"`
	Skype    string `json:"skype,omitempty"`
	QQ       string `json:"qq,omitempty"`
	Weixin   string `json:"weixin,omitempty"`
	Address  string `json:"address,omitempty"`
	Birthday string `json:"birthday,omitempty"`
	Commiter string `json:"commiter,omitempty"`

	// WARN: never logged. These are write-side only — userCtrlWire.toUser
	// leaves them empty so the round-trip can't accidentally surface them.
	Password       string `json:"password,omitempty"`
	VerifyPassword string `json:"verifyPassword,omitempty"`

	// Read-only / server-managed (decoded from GET, not echoed to server).
	Last       string `json:"-"`
	Visits     int    `json:"-"`
	Locked     string `json:"-"`
	Deleted    int    `json:"-"` // 0 = active, 1 = soft-deleted
	ClientLang string `json:"-"`
}

// userCtrlWire mirrors what ZenTao Max 8.1 actually serialises for a
// user inside `user-edit-<id>.json` GET (the read primitive — `view`
// always 302s to todocalendar). Numeric columns flip between native
// int and JSON string across actions, so they ride through json.Number.
type userCtrlWire struct {
	ID         json.Number `json:"id"`
	Account    string      `json:"account"`
	Realname   string      `json:"realname"`
	Email      string      `json:"email"`
	Phone      string      `json:"phone"`
	Mobile     string      `json:"mobile"`
	Dept       json.Number `json:"dept"`
	Role       string      `json:"role"`
	Gender     string      `json:"gender"`
	Visions    string      `json:"visions"`
	Type       string      `json:"type"`
	Nickname   string      `json:"nickname"`
	Skype      string      `json:"skype"`
	QQ         string      `json:"qq"`
	Weixin     string      `json:"weixin"`
	Address    string      `json:"address"`
	Birthday   string      `json:"birthday"`
	Commiter   string      `json:"commiter"`
	Last       string      `json:"last"`
	Visits     json.Number `json:"visits"`
	Locked     string      `json:"locked"`
	Deleted    json.Number `json:"deleted"` // 0/1 — int on Max 8.1, string on some versions
	ClientLang string      `json:"clientLang"`
}

func (w userCtrlWire) toUser() (*User, error) {
	id, err := jsonNumberToInt(w.ID, "id")
	if err != nil {
		return nil, err
	}
	dept, err := jsonNumberToInt(w.Dept, "dept")
	if err != nil {
		return nil, err
	}
	visits, err := jsonNumberToInt(w.Visits, "visits")
	if err != nil {
		return nil, err
	}
	deleted, err := jsonNumberToInt(w.Deleted, "deleted")
	if err != nil {
		return nil, err
	}
	return &User{
		ID:         id,
		Account:    w.Account,
		Realname:   w.Realname,
		Email:      w.Email,
		Phone:      w.Phone,
		Mobile:     w.Mobile,
		Dept:       dept,
		Role:       w.Role,
		Gender:     w.Gender,
		Visions:    w.Visions,
		Type:       w.Type,
		Nickname:   w.Nickname,
		Skype:      w.Skype,
		QQ:         w.QQ,
		Weixin:     w.Weixin,
		Address:    w.Address,
		Birthday:   w.Birthday,
		Commiter:   w.Commiter,
		Last:       w.Last,
		Visits:     visits,
		Locked:     w.Locked,
		Deleted:    deleted,
		ClientLang: w.ClientLang,
		// Password / VerifyPassword left zero on read — see User doc comment.
	}, nil
}

// userPath / usersPath / userEditPath are tiny helpers mirroring the
// productPath / programPath conventions used elsewhere in the package.
func userEditPath(id int) string   { return controllerPath("user", "edit", []string{strconv.Itoa(id)}) }
func userDeletePath(id int) string { return controllerPath("user", "delete", []string{strconv.Itoa(id)}) }

const userCreatePath = "user-create.json"

// userEditInner is the shape ZenTao Max 8.1 returns inside the
// CtrlEnvelope.Data of a `user-edit-<id>.json` GET. Only the `user`
// field matters to us; everything else (depts/groups/visions/companies)
// is form-context for HTML rendering.
type userEditInner struct {
	User json.RawMessage `json:"user"`
}

// GetUser fetches a user by numeric id via the `user-edit-<id>.json`
// GET endpoint. We use edit-GET as the read primitive because
// `user-view-<x>.json` always 302s to `user-todocalendar-<x>.json` on
// this version (probe finding).
//
// Returns ErrNotFound when the inner.user field is absent or returned
// as `false` (the empty-marker shape ZenTao uses when no row matched).
func (c *Client) GetUser(ctx context.Context, id int) (*User, error) {
	body, status, err := c.doController(ctx, "user", "edit", []string{strconv.Itoa(id)}, nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var env CtrlEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode get-user envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner userEditInner
	if err := DecodeData(env, &inner); err != nil {
		return nil, fmt.Errorf("decode get-user data: %w (body=%s)", err, string(body))
	}
	if len(inner.User) == 0 || string(inner.User) == "false" || string(inner.User) == "null" {
		return nil, ErrNotFound
	}
	var wire userCtrlWire
	if err := json.Unmarshal(inner.User, &wire); err != nil {
		return nil, fmt.Errorf("decode get-user wire: %w (body=%s)", err, string(body))
	}
	return wire.toUser()
}

// userToForm encodes a *User into the form-urlencoded shape ZenTao's
// user controller expects on POST. Empty fields are omitted so partial
// edits don't blank server-side defaults — except for the required
// fields, which the caller-level pre-flight has already validated.
//
// `password` and `verifyPassword` ride through verbatim when set;
// callers wanting to omit them just leave the User fields zero.
func userToForm(u *User) url.Values {
	form := url.Values{}
	form.Set("account", u.Account)
	form.Set("realname", u.Realname)
	if u.Password != "" {
		form.Set("password", u.Password)
	}
	if u.VerifyPassword != "" {
		form.Set("verifyPassword", u.VerifyPassword)
	}
	if u.Email != "" {
		form.Set("email", u.Email)
	}
	if u.Phone != "" {
		form.Set("phone", u.Phone)
	}
	if u.Mobile != "" {
		form.Set("mobile", u.Mobile)
	}
	if u.Dept != 0 {
		form.Set("dept", strconv.Itoa(u.Dept))
	}
	if u.Role != "" {
		form.Set("role", u.Role)
	}
	if u.Gender != "" {
		form.Set("gender", u.Gender)
	}
	if u.Type != "" {
		form.Set("type", u.Type)
	}
	if u.Nickname != "" {
		form.Set("nickname", u.Nickname)
	}
	if u.Skype != "" {
		form.Set("skype", u.Skype)
	}
	if u.QQ != "" {
		form.Set("qq", u.QQ)
	}
	if u.Weixin != "" {
		form.Set("weixin", u.Weixin)
	}
	if u.Address != "" {
		form.Set("address", u.Address)
	}
	if u.Birthday != "" {
		form.Set("birthday", u.Birthday)
	}
	if u.Commiter != "" {
		form.Set("commiter", u.Commiter)
	}
	// `visions` is required by the validator; default to "rnd" if the
	// caller didn't pin it.
	if u.Visions != "" {
		form.Set("visions", u.Visions)
	} else {
		form.Set("visions", "rnd")
	}
	return form
}

// CreateUser creates a user via `user-create.json` POST with form-
// urlencoded body. Returns ErrUnauthorized / ErrNotFound where the
// envelope reasons match the standard helpers; otherwise *APIError
// (the license-cap message and verifyPassword sudo failures both flow
// through this path with full reason text intact).
//
// Server-side success carries no id — ZenTao Max 8.1's create returns
// only `{result, message, load}` and the load is a redirect to a list
// page. Until an account-keyed lookup is added, the returned User
// does NOT have ID populated; callers that need it must look up by
// account themselves. The unchanged input User is returned as a
// success indicator only.
func (c *Client) CreateUser(ctx context.Context, u *User) (*User, error) {
	if u == nil {
		return nil, fmt.Errorf("CreateUser: user is nil")
	}
	if u.Account == "" {
		return nil, fmt.Errorf("CreateUser: account required")
	}
	if u.Password == "" {
		return nil, fmt.Errorf("CreateUser: password required")
	}
	if u.Realname == "" {
		return nil, fmt.Errorf("CreateUser: realname required")
	}

	body, status, err := c.doControllerForm(ctx, "user", "create", nil, nil, userToForm(u))
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-user envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	out := *u
	// Sensitive fields don't ride back to the caller — return-side
	// matches the read-side discipline.
	out.Password = ""
	out.VerifyPassword = ""
	return &out, nil
}

// UpdateUser edits a user via `user-edit-<id>.json` POST with form-
// urlencoded body. On success, re-fetches via GetUser to surface the
// authoritative server state (matching the V2 wrapper convention).
//
// Instances with the verifyPassword sudo gate (observed: ZenTao Max
// 8.1) require User.VerifyPassword to be populated; otherwise the
// envelope `{result:fail, message:{verifyPassword:[...]}}` is
// surfaced as *APIError with the field-error map composed into a
// single readable reason.
func (c *Client) UpdateUser(ctx context.Context, u *User) (*User, error) {
	if u == nil {
		return nil, fmt.Errorf("UpdateUser: user is nil")
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("UpdateUser: id required")
	}
	body, status, err := c.doControllerForm(ctx, "user", "edit", []string{strconv.Itoa(u.ID)}, nil, userToForm(u))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode update-user envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	return c.GetUser(ctx, u.ID)
}

// DeleteUser removes a user via `user-delete-<id>.json?confirm=yes`.
// Idempotent on missing rows: HTTP 404, the shape-A "no row" envelope
// (`{status:success, data:"...user:false..."}`), and "not exist"
// envelope reasons all return nil.
//
// Real-row delete envelope shape was not probed (license cap blocked
// creating a disposable user). Both shape A (CtrlEnvelope) and shape
// C (CtrlSimpleResponse) are tolerated — the wrapper tries A first,
// falls back to C, and only surfaces *APIError if neither matches.
func (c *Client) DeleteUser(ctx context.Context, id int) error {
	body, status, err := c.doController(ctx, "user", "delete", []string{strconv.Itoa(id)},
		map[string]string{"confirm": "yes"}, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 400 {
		return apiError(status, body)
	}

	// Shape A first — probe of user-delete-<missing-id> hit this path.
	var env CtrlEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Status != "" {
		if env.Status == "success" {
			return nil
		}
		if isNotFoundReason(env.ZentaoFailReason()) {
			return nil
		}
		return classifyCtrlError(status, env, body)
	}

	// Shape C fallback — successful real delete may use this envelope.
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err == nil && resp.Result != "" {
		if resp.IsSuccess() {
			return nil
		}
		return classifyCtrlSimple(status, resp, body)
	}

	return apiError(status, body)
}

// userEditPath / userDeletePath / userCreatePath are referenced by
// integration tests / future call sites; keep them around even when
// the typed wrappers above don't read them directly.
var _ = userEditPath
var _ = userDeletePath
var _ = userCreatePath
