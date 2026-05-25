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

type programEditInner struct {
	Program json.RawMessage `json:"program"`
}

// Program represents a ZenTao program (project portfolio).
type Program struct {
	ID         *int64  `json:"id,omitempty"`
	Parent     *int64  `json:"parent,omitempty"`
	Path       *string `json:"path,omitempty"`
	Name       *string `json:"name"`
	PM         *string `json:"PM,omitempty"`
	Budget     *string `json:"budget,omitempty"`
	BudgetUnit *string `json:"budgetUnit,omitempty"`
	Begin      *string `json:"begin"`
	End        *string `json:"end"`
	Desc       *string `json:"desc,omitempty"`
	Status     *string `json:"status,omitempty"`
	ACL        *string `json:"acl,omitempty"`
	Whitelist  *string `json:"whitelist,omitempty"`
	Deleted    *bool   `json:"deleted,omitempty"`
}

// UnmarshalJSON decodes a program-edit GET wire payload into *Program.
// ZenTao's controller surface returns id/parent/deleted as a mix of JSON
// numbers and quoted-number strings; the json.Number locals tolerate both
// shapes. Absent fields stay nil so callers can distinguish "wire omitted
// this column" from "wire said empty string".
func (p *Program) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID         json.Number `json:"id"`
		Parent     json.Number `json:"parent"`
		Path       *string     `json:"path"`
		Name       *string     `json:"name"`
		PM         *string     `json:"PM"`
		Budget     *string     `json:"budget"`
		BudgetUnit *string     `json:"budgetUnit"`
		Begin      *string     `json:"begin"`
		End        *string     `json:"end"`
		Desc       *string     `json:"desc"`
		Status     *string     `json:"status"`
		ACL        *string     `json:"acl"`
		Whitelist  *string     `json:"whitelist"`
		Deleted    json.Number `json:"deleted"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != "" {
		id, err := jsonNumberToInt64(raw.ID, "id")
		if err != nil {
			return err
		}
		p.ID = &id
	}
	if raw.Parent != "" {
		parent, err := jsonNumberToInt64(raw.Parent, "parent")
		if err != nil {
			return err
		}
		p.Parent = &parent
	}
	if raw.Deleted != "" {
		d := jsonNumberToBool(raw.Deleted)
		p.Deleted = &d
	}
	p.Path = raw.Path
	p.Name = raw.Name
	p.PM = raw.PM
	p.Budget = raw.Budget
	p.BudgetUnit = raw.BudgetUnit
	p.Begin = raw.Begin
	p.End = raw.End
	p.Desc = raw.Desc
	p.Status = raw.Status
	p.ACL = raw.ACL
	p.Whitelist = raw.Whitelist
	return nil
}

// toForm always emits every form.php writeable field, even when the value
// is empty/0.
func (p *Program) toForm() url.Values {
	form := url.Values{}
	form.Set("parent", strconv.FormatInt(deref(p.Parent), 10))
	form.Set("name", deref(p.Name))
	form.Set("PM", deref(p.PM))
	form.Set("budget", deref(p.Budget))
	form.Set("budgetUnit", deref(p.BudgetUnit))
	form.Set("begin", deref(p.Begin))
	form.Set("end", deref(p.End))
	form.Set("desc", deref(p.Desc))
	form.Set("acl", deref(p.ACL))
	form.Set("whitelist", deref(p.Whitelist))
	form.Set("deleted", boolToIntStr(deref(p.Deleted)))
	return form
}

// mergeProgramBaseline copies baseline and overrides only the fields the
// caller explicitly set on input (non-nil pointers). A nil pointer reads
// as "preserve baseline". This is the M-Z merge that makes UpdateProgram
// safe against ZenTao's non-PATCH semantics.
func mergeProgramBaseline(input, baseline *Program) *Program {
	out := *baseline
	if input.ID != nil {
		out.ID = input.ID
	}
	if input.Parent != nil {
		out.Parent = input.Parent
	}
	if input.Name != nil {
		out.Name = input.Name
	}
	if input.PM != nil {
		out.PM = input.PM
	}
	if input.Budget != nil {
		out.Budget = input.Budget
	}
	if input.BudgetUnit != nil {
		out.BudgetUnit = input.BudgetUnit
	}
	if input.Begin != nil {
		out.Begin = input.Begin
	}
	if input.End != nil {
		out.End = input.End
	}
	if input.Desc != nil {
		out.Desc = input.Desc
	}
	if input.ACL != nil {
		out.ACL = input.ACL
	}
	if input.Whitelist != nil {
		out.Whitelist = input.Whitelist
	}
	if input.Deleted != nil {
		out.Deleted = input.Deleted
	}
	return &out
}

// GetProgram fetches a program via the program-edit-{id} GET endpoint.
// Missing rows — HTTP 404, an envelope-fail "does not exist" reason, or the
// `program:false` payload Max 8.x returns — all collapse to ErrNotFound.
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
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode get-program response: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner programEditInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode get-program data: %w (body=%s)", err, string(body))
	}
	if len(inner.Program) == 0 || string(inner.Program) == "false" || string(inner.Program) == "null" {
		return nil, ErrNotFound
	}
	var prog Program
	if err := json.Unmarshal(inner.Program, &prog); err != nil {
		return nil, fmt.Errorf("decode get-program wire: %w (body=%s)", err, string(body))
	}
	prog.Name = decodeEntitiesPtr(prog.Name)
	return &prog, nil
}

func (c *Client) CreateProgram(ctx context.Context, p *Program) (*Program, error) {
	if p == nil {
		return nil, fmt.Errorf("CreateProgram: program is nil")
	}
	if p.Name == nil {
		return nil, fmt.Errorf("CreateProgram: name required")
	}
	if p.Begin == nil {
		return nil, fmt.Errorf("CreateProgram: begin required")
	}
	if p.End == nil {
		return nil, fmt.Errorf("CreateProgram: end required")
	}
	body, status, err := c.doControllerForm(ctx, "program", "create", nil, nil, p.toForm())
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
		return nil, fmt.Errorf("decode program-create response: %w (body=%s)", err, string(body))
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
	out.ID = &id
	return &out, nil
}

func (c *Client) UpdateProgram(ctx context.Context, p *Program) (*Program, error) {
	if p == nil {
		return nil, fmt.Errorf("UpdateProgram: program is nil")
	}
	if p.ID == nil || *p.ID == 0 {
		return nil, fmt.Errorf("UpdateProgram: id required")
	}
	id := *p.ID
	baseline, err := c.GetProgram(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("UpdateProgram: fetch baseline for merge: %w", err)
	}
	merged := mergeProgramBaseline(p, baseline)
	body, status, err := c.doControllerForm(ctx, "program", "edit", []string{strconv.FormatInt(id, 10)}, nil, merged.toForm())
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
		return nil, fmt.Errorf("decode program-update response: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	return c.GetProgram(ctx, id)
}

// SetProgramParent attaches childID under parentID, or detaches childID
// when parentID is 0. ZenTao silently accepts self-attach and multi-level
// cycles (probe finding F3) — this wrapper rejects both client-side
// before issuing any write.
//
// Validation order is cost-ordered: the zero-cost checks (positive childID,
// non-negative parentID, self-attach) run first; the baseline-dependent
// ancestry-cycle check fetches the prospective parent only when parentID > 0.
// The form-edit POST itself is delegated to UpdateProgram — passing an input
// with only ID and Parent set lets the M-Z merge preserve every other column.
func (c *Client) SetProgramParent(ctx context.Context, childID, parentID int64) (*Program, error) {
	if childID <= 0 {
		return nil, fmt.Errorf("SetProgramParent: childID must be positive, got %d", childID)
	}
	if parentID < 0 {
		return nil, fmt.Errorf("SetProgramParent: parentID cannot be negative, got %d", parentID)
	}
	if parentID == childID {
		return nil, fmt.Errorf("SetProgramParent: %w (self-attach: child=parent=%d)", ErrCycleDetected, childID)
	}
	if parentID > 0 {
		parentRow, err := c.GetProgram(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("SetProgramParent: fetch parent for cycle check: %w", err)
		}
		// zt_project.path is a comma-bracketed ancestry list, e.g. ",1,5,20,".
		// If childID appears as a segment in parentRow.Path, the would-be parent
		// is already a descendant of the child — attaching would form a cycle.
		childToken := "," + strconv.FormatInt(childID, 10) + ","
		parentPath := deref(parentRow.Path)
		if strings.Contains(parentPath, childToken) {
			return nil, fmt.Errorf("SetProgramParent: %w (child=%d is ancestor of parent=%d, parent.path=%q)",
				ErrCycleDetected, childID, parentID, parentPath)
		}
	}
	updated, err := c.UpdateProgram(ctx, &Program{ID: &childID, Parent: &parentID})
	if err != nil {
		return nil, fmt.Errorf("SetProgramParent: %w", err)
	}
	return updated, nil
}

func (c *Client) DeleteProgram(ctx context.Context, id int64) error {
	body, status, err := c.doController(ctx, "program", "delete", []string{strconv.FormatInt(id, 10), "yes"}, nil, nil)
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
		return fmt.Errorf("decode program-delete response: %w (body=%s)", err, string(body))
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
