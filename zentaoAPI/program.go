package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

func programToForm(p *Program) url.Values {
	form := url.Values{}
	form.Set("name", p.Name)
	form.Set("begin", p.Begin)
	form.Set("end", p.End)
	if p.Parent != 0 {
		form.Set("parent", strconv.Itoa(p.Parent))
	}
	if p.PM != "" {
		form.Set("PM", p.PM)
	}
	if p.Desc != "" {
		form.Set("desc", p.Desc)
	}
	if p.ACL != "" {
		form.Set("acl", p.ACL)
	}
	if p.Budget != "" {
		form.Set("budget", p.Budget)
	}
	if p.BudgetUnit != "" {
		form.Set("budgetUnit", p.BudgetUnit)
	}
	if p.Whitelist != "" {
		form.Set("whitelist", p.Whitelist)
	}
	return form
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
	body, status, err := c.doControllerForm(ctx, "program", "edit", []string{strconv.Itoa(p.ID)}, nil, programToForm(p))
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
