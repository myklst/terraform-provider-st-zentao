package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// Program is the canonical in-memory representation of a ZenTao program
// (a.k.a. project portfolio). Same conventions as Product: fields without
// json:"-" go on the wire for POST/PUT; fields with json:"-" are decoded
// from GET responses but never echoed back to the server.
type Program struct {
	// Identity & content
	ID    int    `json:"id,omitempty"`
	Name  string `json:"name"`
	Begin string `json:"begin"` // YYYY-MM-DD
	End   string `json:"end"`   // YYYY-MM-DD

	// Optional writeable fields per v2 docs (POST/PUT body).
	PM          string `json:"PM,omitempty"`
	Description string `json:"desc,omitempty"`

	// Read-only / server-managed (decoded from GET, not sent on write).
	Code         string `json:"-"`
	Status       string `json:"-"`
	Parent       int    `json:"-"`
	Type         string `json:"-"`
	Category     string `json:"-"`
	ACL          string `json:"-"`
	PO           string `json:"-"`
	QD           string `json:"-"`
	RD           string `json:"-"`
	Budget       string `json:"-"`
	BudgetUnit   string `json:"-"`
	OpenedBy     string `json:"-"`
	OpenedDate   string `json:"-"`
	LastEditedBy string `json:"-"`
	RealBegan    string `json:"-"`
	RealEnd      string `json:"-"`
	Progress     string `json:"-"`
	TeamCount    string `json:"-"`
}

// programV2Wire mirrors Product's wire approach: every numeric column that
// v2 serializes as a JSON string gets decoded through json.Number.
type programV2Wire struct {
	ID           json.Number `json:"id"`
	Name         string      `json:"name"`
	Code         string      `json:"code"`
	Begin        string      `json:"begin"`
	End          string      `json:"end"`
	Status       string      `json:"status"`
	Parent       json.Number `json:"parent"`
	Type         string      `json:"type"`
	Category     string      `json:"category"`
	Description  string      `json:"desc"`
	PM           string      `json:"PM"`
	PO           string      `json:"PO"`
	QD           string      `json:"QD"`
	RD           string      `json:"RD"`
	ACL          string      `json:"acl"`
	Budget       string      `json:"budget"`
	BudgetUnit   string      `json:"budgetUnit"`
	OpenedBy     string      `json:"openedBy"`
	OpenedDate   string      `json:"openedDate"`
	LastEditedBy string      `json:"lastEditedBy"`
	RealBegan    string      `json:"realBegan"`
	RealEnd      string      `json:"realEnd"`
	Progress     string      `json:"progress"`
	TeamCount    string      `json:"teamCount"`
}

func (w programV2Wire) toProgram() (*Program, error) {
	id, err := jsonNumberToInt(w.ID, "id")
	if err != nil {
		return nil, err
	}
	parent, err := jsonNumberToInt(w.Parent, "parent")
	if err != nil {
		return nil, err
	}
	return &Program{
		ID:           id,
		Name:         w.Name,
		Code:         w.Code,
		Begin:        w.Begin,
		End:          w.End,
		Status:       w.Status,
		Parent:       parent,
		Type:         w.Type,
		Category:     w.Category,
		Description:  w.Description,
		PM:           w.PM,
		PO:           w.PO,
		QD:           w.QD,
		RD:           w.RD,
		ACL:          w.ACL,
		Budget:       w.Budget,
		BudgetUnit:   w.BudgetUnit,
		OpenedBy:     w.OpenedBy,
		OpenedDate:   w.OpenedDate,
		LastEditedBy: w.LastEditedBy,
		RealBegan:    w.RealBegan,
		RealEnd:      w.RealEnd,
		Progress:     w.Progress,
		TeamCount:    w.TeamCount,
	}, nil
}

func programPath(id int) string {
	return "api.php/v2/programs/" + strconv.Itoa(id)
}

const programsPath = "api.php/v2/programs"

// GetProgram fetches a program by ID via GET /api.php/v2/programs/{id}.
// Returns ErrNotFound on HTTP 404 OR on the {"status":"fail","message":
// "...does not exist..."} shape v2 emits at HTTP 200.
func (c *Client) GetProgram(ctx context.Context, id int) (*Program, error) {
	body, status, err := c.doV2Request(ctx, http.MethodGet, programPath(id), nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp struct {
		ZentaoResponse
		Program programV2Wire `json:"program"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode get-program: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		if isNotFoundReason(resp.ZentaoFailReason()) {
			return nil, ErrNotFound
		}
		return nil, apiError(status, body)
	}
	return resp.Program.toProgram()
}

// CreateProgram creates a program via POST /api.php/v2/programs. v2 only
// echoes back the new id; the caller should re-fetch via GetProgram if it
// needs server-defaulted fields like opened_by / status / parent.
func (c *Client) CreateProgram(ctx context.Context, p *Program) (*Program, error) {
	body, status, err := c.doV2Request(ctx, http.MethodPost, programsPath, nil, p)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp struct {
		ZentaoResponse
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-program: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		return nil, apiError(status, body)
	}
	id, _ := resp.ID.Int64()
	if id == 0 {
		return nil, fmt.Errorf("create program: empty id in response (body=%s)", string(body))
	}
	out := *p
	out.ID = int(id)
	return &out, nil
}

// UpdateProgram edits a program via PUT /api.php/v2/programs/{id}. v2 only
// returns {status}, so on success we re-fetch the program via GetProgram
// to surface authoritative state to the caller.
func (c *Client) UpdateProgram(ctx context.Context, p *Program) (*Program, error) {
	if p.ID == 0 {
		return nil, fmt.Errorf("UpdateProgram: missing id")
	}
	body, status, err := c.doV2Request(ctx, http.MethodPut, programPath(p.ID), nil, p)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp ZentaoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode update-program: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		if isNotFoundReason(resp.ZentaoFailReason()) {
			return nil, ErrNotFound
		}
		return nil, apiError(status, body)
	}
	return c.GetProgram(ctx, p.ID)
}

// DeleteProgram removes a program via DELETE /api.php/v2/programs/{id}.
// Idempotent on missing rows (HTTP 404 OR HTTP 200 + "does not exist").
func (c *Client) DeleteProgram(ctx context.Context, id int) error {
	body, status, err := c.doV2Request(ctx, http.MethodDelete, programPath(id), nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 400 {
		return apiError(status, body)
	}
	var resp ZentaoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode delete-program: %w (body=%s)", err, string(body))
	}
	if resp.Status == "success" {
		return nil
	}
	if isNotFoundReason(resp.ZentaoFailReason()) {
		return nil
	}
	return apiError(status, body)
}
