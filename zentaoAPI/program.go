package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Program represents a ZenTao program (project portfolio).
//
// Path is server-derived (zt_project.path, comma-bracketed ancestry list,
// e.g. ",1,5,10,") and is read-only — programToForm never emits it.
type Program struct {
	ID         int64  `json:"-"`
	Parent     int64  `json:"parent,omitempty"`
	Path       string `json:"-"`
	Name       string `json:"name"`
	PM         string `json:"PM,omitempty"`
	Budget     string `json:"budget,omitempty"`
	BudgetUnit string `json:"budgetUnit,omitempty"`
	Begin      string `json:"begin"`
	End        string `json:"end"`
	Desc       string `json:"desc,omitempty"`
	Status     string `json:"status"`
	ACL        string `json:"acl,omitempty"`
	Whitelist  string `json:"whitelist,omitempty"`
}

type programCtrlWire struct {
	ID         json.Number `json:"id"`
	Parent     json.Number `json:"parent"`
	Path       string      `json:"path"`
	Name       string      `json:"name"`
	PM         string      `json:"PM"`
	Budget     string      `json:"budget"`
	BudgetUnit string      `json:"budgetUnit"`
	Begin      string      `json:"begin"`
	End        string      `json:"end"`
	Desc       string      `json:"desc"`
	Status     string      `json:"status"`
	ACL        string      `json:"acl"`
	Whitelist  string      `json:"whitelist"`
	Deleted    json.Number `json:"deleted"`
}

func (w programCtrlWire) toProgram() (*Program, error) {
	id, err := jsonNumberToInt64(w.ID, "id")
	if err != nil {
		return nil, err
	}
	parent, err := jsonNumberToInt64(w.Parent, "parent")
	if err != nil {
		return nil, err
	}
	return &Program{
		ID:         id,
		Parent:     parent,
		Path:       w.Path,
		Name:       w.Name,
		PM:         w.PM,
		Budget:     w.Budget,
		BudgetUnit: w.BudgetUnit,
		Begin:      w.Begin,
		End:        w.End,
		Desc:       w.Desc,
		Status:     w.Status,
		ACL:        w.ACL,
		Whitelist:  w.Whitelist,
	}, nil
}

type programEditInner struct {
	Program json.RawMessage `json:"program"`
}

// programToForm always emits every form.php writeable field, even when the
// value is empty/0. ZenTao's program-edit POST is not PATCH-semantic — any
// omitted form.php field is reset to its form.php default. See
// docs/superpowers/specs/probe-program-controller.md §8.
func programToForm(p *Program) url.Values {
	form := url.Values{}
	form.Set("parent", strconv.FormatInt(p.Parent, 10))
	form.Set("name", p.Name)
	form.Set("PM", p.PM)
	form.Set("budget", p.Budget)
	form.Set("budgetUnit", p.BudgetUnit)
	form.Set("begin", p.Begin)
	form.Set("end", p.End)
	form.Set("desc", p.Desc)
	form.Set("acl", p.ACL)
	form.Set("whitelist", p.Whitelist)
	return form
}

// mergeProgramBaseline copies baseline and overrides only the fields the
// caller explicitly set on input (non-zero / non-empty). Empty string and 0
// are read as "preserve baseline". This is the M-Z merge that makes
// UpdateProgram safe against ZenTao's non-PATCH semantics.
func mergeProgramBaseline(input, baseline *Program) *Program {
	out := *baseline
	out.ID = input.ID
	if input.Parent != 0 {
		out.Parent = input.Parent
	}
	if input.Name != "" {
		out.Name = input.Name
	}
	if input.PM != "" {
		out.PM = input.PM
	}
	if input.Budget != "" {
		out.Budget = input.Budget
	}
	if input.BudgetUnit != "" {
		out.BudgetUnit = input.BudgetUnit
	}
	if input.Begin != "" {
		out.Begin = input.Begin
	}
	if input.End != "" {
		out.End = input.End
	}
	if input.Desc != "" {
		out.Desc = input.Desc
	}
	if input.ACL != "" {
		out.ACL = input.ACL
	}
	if input.Whitelist != "" {
		out.Whitelist = input.Whitelist
	}
	return &out
}

func (c *Client) GetProgram(ctx context.Context, id int64) (*Program, error) {
	body, status, err := c.doController(ctx, "program", "edit", []string{strconv.FormatInt(id, 10)}, nil, nil)
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
		return nil, fmt.Errorf("decode get-program envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner programEditInner
	if err := DecodeData(env, &inner); err != nil {
		return nil, fmt.Errorf("decode get-program data: %w (body=%s)", err, string(body))
	}
	if len(inner.Program) == 0 || string(inner.Program) == "false" || string(inner.Program) == "null" {
		return nil, ErrNotFound
	}
	var wire programCtrlWire
	if err := json.Unmarshal(inner.Program, &wire); err != nil {
		return nil, fmt.Errorf("decode get-program wire: %w (body=%s)", err, string(body))
	}
	// Soft-deleted rows still come back from edit-GET (probe 2026-05-09:
	// DELETE only flips zt_project.deleted=1, the row is not removed).
	// Surface them as gone so Terraform Read clears state.
	if wire.Deleted.String() == "1" {
		return nil, ErrNotFound
	}
	out, err := wire.toProgram()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateProgram(ctx context.Context, p *Program) (*Program, error) {
	if p == nil {
		return nil, fmt.Errorf("CreateProgram: program is nil")
	}
	if p.Name == "" {
		return nil, fmt.Errorf("CreateProgram: name required")
	}
	if p.Begin == "" {
		return nil, fmt.Errorf("CreateProgram: begin required")
	}
	if p.End == "" {
		return nil, fmt.Errorf("CreateProgram: end required")
	}
	body, status, err := c.doControllerForm(ctx, "program", "create", nil, nil, programToForm(p))
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp struct {
		Result  string          `json:"result"`
		Message json.RawMessage `json:"message,omitempty"`
		ID      json.Number     `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-program: %w (body=%s)", err, string(body))
	}
	if resp.Result != "success" {
		var simple CtrlSimpleResponse
		_ = json.Unmarshal(body, &simple)
		return nil, classifyCtrlSimple(status, simple, body)
	}
	id, _ := resp.ID.Int64()
	if id == 0 {
		return nil, fmt.Errorf("create program: empty id in response (body=%s)", string(body))
	}
	out := *p
	out.ID = id
	return &out, nil
}

func (c *Client) UpdateProgram(ctx context.Context, p *Program) (*Program, error) {
	if p == nil {
		return nil, fmt.Errorf("UpdateProgram: program is nil")
	}
	if p.ID == 0 {
		return nil, fmt.Errorf("UpdateProgram: id required")
	}
	baseline, err := c.GetProgram(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("UpdateProgram: fetch baseline for merge: %w", err)
	}
	merged := mergeProgramBaseline(p, baseline)
	body, status, err := c.doControllerForm(ctx, "program", "edit", []string{strconv.FormatInt(p.ID, 10)}, nil, programToForm(merged))
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
		return nil, fmt.Errorf("decode update-program envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	return c.GetProgram(ctx, p.ID)
}

// SetProgramParent attaches childID under parentID, or detaches childID
// when parentID is 0. ZenTao silently accepts self-attach and multi-level
// cycles (probe finding F3) — this wrapper rejects both client-side
// before issuing the form POST.
//
// Implementation: fetch the child as baseline, override only the Parent
// field (the rest of the program-edit form is preserved verbatim — see
// the M-Z merge note on UpdateProgram), then submit. programToForm
// always-sets parent so a parentID of 0 actually clears the column.
func (c *Client) SetProgramParent(ctx context.Context, childID, parentID int64) error {
	if childID <= 0 {
		return fmt.Errorf("SetProgramParent: childID must be positive, got %d", childID)
	}
	if parentID < 0 {
		return fmt.Errorf("SetProgramParent: parentID cannot be negative, got %d", parentID)
	}
	if parentID == childID {
		return fmt.Errorf("SetProgramParent: %w (self-attach: child=parent=%d)", ErrCycleDetected, childID)
	}
	baseline, err := c.GetProgram(ctx, childID)
	if err != nil {
		return fmt.Errorf("SetProgramParent: fetch child baseline: %w", err)
	}
	if parentID > 0 {
		parentRow, err := c.GetProgram(ctx, parentID)
		if err != nil {
			return fmt.Errorf("SetProgramParent: fetch parent for cycle check: %w", err)
		}
		// zt_project.path is a comma-bracketed ancestry list, e.g. ",1,5,20,".
		// If childID appears as a segment in parentRow.Path, the would-be parent
		// is already a descendant of the child — attaching would form a cycle.
		childToken := "," + strconv.FormatInt(childID, 10) + ","
		if strings.Contains(parentRow.Path, childToken) {
			return fmt.Errorf("SetProgramParent: %w (child=%d is ancestor of parent=%d, parent.path=%q)",
				ErrCycleDetected, childID, parentID, parentRow.Path)
		}
	}
	out := *baseline
	out.Parent = parentID
	body, status, err := c.doControllerForm(ctx, "program", "edit", []string{strconv.FormatInt(childID, 10)}, nil, programToForm(&out))
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= 400 {
		return apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode set-program-parent envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return classifyCtrlSimple(status, resp, body)
	}
	return nil
}

func (c *Client) DeleteProgram(ctx context.Context, id int) error {
	body, status, err := c.doController(ctx, "program", "delete", []string{strconv.Itoa(id), "yes"}, nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 400 {
		return apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode delete-program envelope: %w (body=%s)", err, string(body))
	}
	if resp.IsSuccess() {
		return nil
	}
	flat, _ := resp.FieldErrors()
	if isNotFoundReason(flat) {
		return nil
	}
	return classifyCtrlSimple(status, resp, body)
}
