package zentaoapi

import "encoding/json"

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
	Deleted    string `json:"-"`
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
	Deleted    string      `json:"deleted"`
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
		Deleted:    w.Deleted,
		ClientLang: w.ClientLang,
		// Password / VerifyPassword left zero on read — see User doc comment.
	}, nil
}
