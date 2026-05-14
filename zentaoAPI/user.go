package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// User represents a ZenTao user. Password / VerifyPassword are
// write-only: the server's hash is never round-tripped onto User.
type User struct {
	ID      int64  `json:"-"`
	Account string `json:"account"`

	Realname string `json:"realname"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Mobile   string `json:"mobile,omitempty"`
	Dept     int64  `json:"dept,omitempty"`
	Role     string `json:"role,omitempty"`
	Gender   string `json:"gender,omitempty"`
	Visions  string `json:"visions,omitempty"`
	Type     string `json:"type,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Skype    string `json:"skype,omitempty"`
	QQ       string `json:"qq,omitempty"`
	Weixin   string `json:"weixin,omitempty"`
	Address  string `json:"address,omitempty"`
	Birthday string `json:"birthday,omitempty"`
	Commiter string `json:"commiter,omitempty"`

	// Write-only; never round-tripped on read.
	Password       string `json:"password,omitempty"`
	VerifyPassword string `json:"verifyPassword,omitempty"`

	Last       string `json:"-"`
	Visits     int64  `json:"-"`
	Locked     string `json:"-"`
	Deleted    int64  `json:"-"`
	ClientLang string `json:"-"`
}

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
	Deleted    json.Number `json:"deleted"`
	ClientLang string      `json:"clientLang"`
}

func (w userCtrlWire) toUser() (*User, error) {
	id, err := jsonNumberToInt64(w.ID, "id")
	if err != nil {
		return nil, err
	}
	dept, err := jsonNumberToInt64(w.Dept, "dept")
	if err != nil {
		return nil, err
	}
	visits, err := jsonNumberToInt64(w.Visits, "visits")
	if err != nil {
		return nil, err
	}
	deleted, err := jsonNumberToInt64(w.Deleted, "deleted")
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
	}, nil
}

func userEditPath(id int) string { return controllerPath("user", "edit", []string{strconv.Itoa(id)}) }
func userDeletePath(id int) string {
	return controllerPath("user", "delete", []string{strconv.Itoa(id)})
}

const userCreatePath = "user-create.json"

type userEditInner struct {
	User json.RawMessage `json:"user"`
}

func (c *Client) GetUser(ctx context.Context, id int64) (*User, error) {
	body, status, err := c.doController(ctx, "user", "edit", []string{strconv.FormatInt(id, 10)}, nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode get-user envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner userEditInner
	if err := env.DecodeData(&inner); err != nil {
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
		form.Set("dept", strconv.FormatInt(u.Dept, 10))
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
	if u.Visions != "" {
		form.Set("visions", u.Visions)
	} else {
		form.Set("visions", "rnd")
	}
	return form
}

// CreateUser does not populate the returned User.ID; the create endpoint
// does not echo it.
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
	out.Password = ""
	out.VerifyPassword = ""
	return &out, nil
}

func (c *Client) UpdateUser(ctx context.Context, u *User) (*User, error) {
	if u == nil {
		return nil, fmt.Errorf("UpdateUser: user is nil")
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("UpdateUser: id required")
	}
	body, status, err := c.doControllerForm(ctx, "user", "edit", []string{strconv.FormatInt(u.ID, 10)}, nil, userToForm(u))
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

	// Try CtrlEnvelope first, then fall back to CtrlSimpleResponse.
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err == nil && env.Status != "" {
		if env.Status == "success" {
			return nil
		}
		if isNotFoundReason(env.ZentaoFailReason()) {
			return nil
		}
		return classifyCtrlError(status, env, body)
	}

	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err == nil && resp.Result != "" {
		if resp.IsSuccess() {
			return nil
		}
		return classifyCtrlSimple(status, resp, body)
	}

	return apiError(status, body)
}

// referenced by integration tests / future call sites.
var _ = userEditPath
var _ = userDeletePath
var _ = userCreatePath
