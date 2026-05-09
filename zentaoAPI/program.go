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
type Program struct {
	ID         int    `json:"-"`
	Name       string `json:"name"`
	Begin      string `json:"begin"`
	End        string `json:"end"`
	Parent     int    `json:"parent,omitempty"`
	PM         string `json:"PM,omitempty"`
	Desc       string `json:"desc,omitempty"`
	ACL        string `json:"acl,omitempty"`
	Budget     string `json:"budget,omitempty"`
	BudgetUnit string `json:"budgetUnit,omitempty"`
	Whitelist  string `json:"whitelist,omitempty"`

	Code           string `json:"-"`
	Status         string `json:"-"`
	Type           string `json:"-"`
	Category       string `json:"-"`
	Lifetime       string `json:"-"`
	Vision         string `json:"-"`
	Attribute      string `json:"-"`
	Model          string `json:"-"`
	Path           string `json:"-"`
	Grade          int    `json:"-"`
	Multiple       string `json:"-"`
	Parallel       string `json:"-"`
	Enabled        string `json:"-"`
	Frozen         string `json:"-"`
	Deleted        string `json:"-"`
	HasProduct     int    `json:"-"`
	WorkflowGroup  int    `json:"-"`
	StoryType      string `json:"-"`
	Pri            int    `json:"-"`
	Version        int    `json:"-"`
	ParentVersion  int    `json:"-"`
	Days           int    `json:"-"`
	FirstEnd       string `json:"-"`
	SubStatus      string `json:"-"`
	OpenedBy       string `json:"-"`
	OpenedDate     string `json:"-"`
	LastEditedBy   string `json:"-"`
	LastEditedDate string `json:"-"`
	RealBegan      string `json:"-"`
	RealEnd        string `json:"-"`
	ClosedBy       string `json:"-"`
	ClosedDate     string `json:"-"`
	ClosedReason   string `json:"-"`
	CanceledBy     string `json:"-"`
	CanceledDate   string `json:"-"`
	SuspendedDate  string `json:"-"`
	PO             string `json:"-"`
	QD             string `json:"-"`
	RD             string `json:"-"`
	Team           string `json:"-"`
	Order          int    `json:"-"`
	Progress       string `json:"-"`
	Percent        string `json:"-"`
	Estimate       string `json:"-"`
	Consumed       string `json:"-"`
	Left           string `json:"-"`
	TeamCount      int    `json:"-"`
}

type programCtrlWire struct {
	ID             json.Number `json:"id"`
	Name           string      `json:"name"`
	Code           string      `json:"code"`
	Begin          string      `json:"begin"`
	End            string      `json:"end"`
	Parent         json.Number `json:"parent"`
	Status         string      `json:"status"`
	Type           string      `json:"type"`
	Category       string      `json:"category"`
	Lifetime       string      `json:"lifetime"`
	Vision         string      `json:"vision"`
	Attribute      string      `json:"attribute"`
	Model          string      `json:"model"`
	Path           string      `json:"path"`
	Grade          json.Number `json:"grade"`
	Multiple       json.Number `json:"multiple"`
	Parallel       json.Number `json:"parallel"`
	Enabled        string      `json:"enabled"`
	Frozen         string      `json:"frozen"`
	Deleted        json.Number `json:"deleted"`
	HasProduct     json.Number `json:"hasProduct"`
	WorkflowGroup  json.Number `json:"workflowGroup"`
	StoryType      string      `json:"storyType"`
	Pri            json.Number `json:"pri"`
	Version        json.Number `json:"version"`
	ParentVersion  json.Number `json:"parentVersion"`
	Days           json.Number `json:"days"`
	FirstEnd       string      `json:"firstEnd"`
	SubStatus      string      `json:"subStatus"`
	Desc           string      `json:"desc"`
	ACL            string      `json:"acl"`
	Whitelist      string      `json:"whitelist"`
	Budget         string      `json:"budget"`
	BudgetUnit     string      `json:"budgetUnit"`
	OpenedBy       string      `json:"openedBy"`
	OpenedDate     string      `json:"openedDate"`
	LastEditedBy   string      `json:"lastEditedBy"`
	LastEditedDate string      `json:"lastEditedDate"`
	RealBegan      string      `json:"realBegan"`
	RealEnd        string      `json:"realEnd"`
	ClosedBy       string      `json:"closedBy"`
	ClosedDate     string      `json:"closedDate"`
	ClosedReason   string      `json:"closedReason"`
	CanceledBy     string      `json:"canceledBy"`
	CanceledDate   string      `json:"canceledDate"`
	SuspendedDate  string      `json:"suspendedDate"`
	PM             string      `json:"PM"`
	PO             string      `json:"PO"`
	QD             string      `json:"QD"`
	RD             string      `json:"RD"`
	Team           string      `json:"team"`
	Order          json.Number `json:"order"`
	Progress       string      `json:"progress"`
	Percent        string      `json:"percent"`
	Estimate       string      `json:"estimate"`
	Consumed       string      `json:"consumed"`
	Left           string      `json:"left"`
	TeamCount      json.Number `json:"teamCount"`
}

func (w programCtrlWire) toProgram() (*Program, error) {
	id, err := jsonNumberToInt(w.ID, "id")
	if err != nil {
		return nil, err
	}
	parent, err := jsonNumberToInt(w.Parent, "parent")
	if err != nil {
		return nil, err
	}
	grade, _ := jsonNumberToInt(w.Grade, "grade")
	hasProduct, _ := jsonNumberToInt(w.HasProduct, "hasProduct")
	workflowGroup, _ := jsonNumberToInt(w.WorkflowGroup, "workflowGroup")
	pri, _ := jsonNumberToInt(w.Pri, "pri")
	version, _ := jsonNumberToInt(w.Version, "version")
	parentVersion, _ := jsonNumberToInt(w.ParentVersion, "parentVersion")
	days, _ := jsonNumberToInt(w.Days, "days")
	order, _ := jsonNumberToInt(w.Order, "order")
	teamCount, _ := jsonNumberToInt(w.TeamCount, "teamCount")
	return &Program{
		ID:             id,
		Name:           w.Name,
		Code:           w.Code,
		Begin:          w.Begin,
		End:            w.End,
		Parent:         parent,
		Status:         w.Status,
		Type:           w.Type,
		Category:       w.Category,
		Lifetime:       w.Lifetime,
		Vision:         w.Vision,
		Attribute:      w.Attribute,
		Model:          w.Model,
		Path:           w.Path,
		Grade:          grade,
		Multiple:       w.Multiple.String(),
		Parallel:       w.Parallel.String(),
		Enabled:        w.Enabled,
		Frozen:         w.Frozen,
		Deleted:        w.Deleted.String(),
		HasProduct:     hasProduct,
		WorkflowGroup:  workflowGroup,
		StoryType:      w.StoryType,
		Pri:            pri,
		Version:        version,
		ParentVersion:  parentVersion,
		Days:           days,
		FirstEnd:       w.FirstEnd,
		SubStatus:      w.SubStatus,
		Desc:           w.Desc,
		ACL:            w.ACL,
		Whitelist:      w.Whitelist,
		Budget:         w.Budget,
		BudgetUnit:     w.BudgetUnit,
		OpenedBy:       w.OpenedBy,
		OpenedDate:     w.OpenedDate,
		LastEditedBy:   w.LastEditedBy,
		LastEditedDate: w.LastEditedDate,
		RealBegan:      w.RealBegan,
		RealEnd:        w.RealEnd,
		ClosedBy:       w.ClosedBy,
		ClosedDate:     w.ClosedDate,
		ClosedReason:   w.ClosedReason,
		CanceledBy:     w.CanceledBy,
		CanceledDate:   w.CanceledDate,
		SuspendedDate:  w.SuspendedDate,
		PM:             w.PM,
		PO:             w.PO,
		QD:             w.QD,
		RD:             w.RD,
		Team:           w.Team,
		Order:          order,
		Progress:       w.Progress,
		Percent:        w.Percent,
		Estimate:       w.Estimate,
		Consumed:       w.Consumed,
		Left:           w.Left,
		TeamCount:      teamCount,
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
	form.Set("name", p.Name)
	form.Set("begin", p.Begin)
	form.Set("end", p.End)
	form.Set("parent", strconv.Itoa(p.Parent))
	form.Set("PM", p.PM)
	form.Set("desc", p.Desc)
	form.Set("acl", p.ACL)
	form.Set("budget", p.Budget)
	form.Set("budgetUnit", p.BudgetUnit)
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
	if input.Name != "" {
		out.Name = input.Name
	}
	if input.Begin != "" {
		out.Begin = input.Begin
	}
	if input.End != "" {
		out.End = input.End
	}
	if input.Parent != 0 {
		out.Parent = input.Parent
	}
	if input.PM != "" {
		out.PM = input.PM
	}
	if input.Desc != "" {
		out.Desc = input.Desc
	}
	if input.ACL != "" {
		out.ACL = input.ACL
	}
	if input.Budget != "" {
		out.Budget = input.Budget
	}
	if input.BudgetUnit != "" {
		out.BudgetUnit = input.BudgetUnit
	}
	if input.Whitelist != "" {
		out.Whitelist = input.Whitelist
	}
	return &out
}

func (c *Client) GetProgram(ctx context.Context, id int) (*Program, error) {
	body, status, err := c.doController(ctx, "program", "edit", []string{strconv.Itoa(id)}, nil, nil)
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
	out, err := wire.toProgram()
	if err != nil {
		return nil, err
	}
	// Soft-deleted rows still come back from edit-GET; treat as gone.
	if out.Deleted == "1" {
		return nil, ErrNotFound
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
	out.ID = int(id)
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
	body, status, err := c.doControllerForm(ctx, "program", "edit", []string{strconv.Itoa(p.ID)}, nil, programToForm(merged))
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
func (c *Client) SetProgramParent(ctx context.Context, childID, parentID int) error {
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
		// path is comma-delimited ancestry (e.g. ",1,5,7,"). If childID
		// already sits in parent's lineage, attaching would form a cycle.
		if strings.Contains(parentRow.Path, fmt.Sprintf(",%d,", childID)) {
			return fmt.Errorf("SetProgramParent: %w (parent %d has child %d in path %q)", ErrCycleDetected, parentID, childID, parentRow.Path)
		}
	}
	out := *baseline
	out.Parent = parentID
	body, status, err := c.doControllerForm(ctx, "program", "edit", []string{strconv.Itoa(childID)}, nil, programToForm(&out))
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
