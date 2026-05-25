package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type projectViewInner struct {
	Project  json.RawMessage            `json:"project"`
	Products map[string]json.RawMessage `json:"products"`
}

// Project represents a ZenTao project
type Project struct {
	ID            *int64   `json:"id,omitempty"`
	Name          *string  `json:"name,omitempty"`
	Model         *string  `json:"model,omitempty"`
	Begin         *string  `json:"begin,omitempty"`
	End           *string  `json:"end,omitempty"`
	Parent        *int64   `json:"parent,omitempty"` // wire: program parent id (0 = top-level)
	Products      *[]int64 `json:"-"`                // joined via zt_projectproduct
	WorkflowGroup *int64   `json:"workflowGroup,omitempty"`
	Multiple      *bool    `json:"multiple,omitempty"` // CREATE-ONLY; ignored by edit POST
	Status        *string  `json:"-"`                  // READ-ONLY; lifecycle column, edited via project-start/suspend/close
	ACL           *string  `json:"acl,omitempty"`
	PM            *string  `json:"PM,omitempty"`
	PO            *string  `json:"PO,omitempty"`
	QD            *string  `json:"QD,omitempty"`
	RD            *string  `json:"RD,omitempty"`
	Desc          *string  `json:"desc,omitempty"`
	Deleted       *bool    `json:"deleted,omitempty"`
}

// UnmarshalJSON decodes the inner `.project` row of `project-view-{id}.json`.
// ZenTao's controller surface returns id/parent/workflowGroup/multiple/deleted
// as a mix of JSON numbers and quoted-number strings; the json.Number locals
// tolerate both shapes. Absent fields stay nil so callers can distinguish
// "wire omitted this column" from "wire said empty string".
//
// Note: Products is NOT decoded here — it lives at outer `.data.products`,
// alongside (not inside) `.project`. GetProject splices it in after this
// decode finishes.
func (p *Project) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID            json.Number `json:"id"`
		Parent        json.Number `json:"parent"`
		WorkflowGroup json.Number `json:"workflowGroup"`
		Multiple      json.Number `json:"multiple"`
		Deleted       json.Number `json:"deleted"`
		Name          *string     `json:"name"`
		Model         *string     `json:"model"`
		Begin         *string     `json:"begin"`
		End           *string     `json:"end"`
		Status        *string     `json:"status"`
		ACL           *string     `json:"acl"`
		PM            *string     `json:"PM"`
		PO            *string     `json:"PO"`
		QD            *string     `json:"QD"`
		RD            *string     `json:"RD"`
		Desc          *string     `json:"desc"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != "" {
		v, err := jsonNumberToInt64(raw.ID, "id")
		if err != nil {
			return err
		}
		p.ID = &v
	}
	if raw.Parent != "" {
		v, err := jsonNumberToInt64(raw.Parent, "parent")
		if err != nil {
			return err
		}
		p.Parent = &v
	}
	if raw.WorkflowGroup != "" {
		v, err := jsonNumberToInt64(raw.WorkflowGroup, "workflowGroup")
		if err != nil {
			return err
		}
		p.WorkflowGroup = &v
	}
	if raw.Multiple != "" {
		v := jsonNumberToBool(raw.Multiple)
		p.Multiple = &v
	}
	if raw.Deleted != "" {
		v := jsonNumberToBool(raw.Deleted)
		p.Deleted = &v
	}
	p.Name = raw.Name
	p.Model = raw.Model
	p.Begin = raw.Begin
	p.End = raw.End
	p.Status = raw.Status
	p.ACL = raw.ACL
	p.PM = raw.PM
	p.PO = raw.PO
	p.QD = raw.QD
	p.RD = raw.RD
	p.Desc = raw.Desc
	return nil
}

// toForm produces a form for `/project-create.json` and `/project-edit.json`
func (p *Project) toForm() url.Values {
	form := url.Values{}
	form.Set("name", deref(p.Name))
	form.Set("model", deref(p.Model))
	form.Set("begin", deref(p.Begin))
	form.Set("end", deref(p.End))
	form.Set("parent", strconv.FormatInt(deref(p.Parent), 10))
	form.Set("workflowGroup", strconv.FormatInt(deref(p.WorkflowGroup), 10))
	form.Set("multiple", boolToOnOff(deref(p.Multiple)))  // uses HTML-checkbox semantics (`on`/`off`), NOT `0`/`1`
	form.Set("acl", deref(p.ACL))
	form.Set("PM", deref(p.PM))
	form.Set("PO", deref(p.PO))
	form.Set("QD", deref(p.QD))
	form.Set("RD", deref(p.RD))
	form.Set("desc", deref(p.Desc))
	form.Set("deleted", boolToIntStr(deref(p.Deleted)))
	// products[] — empty list still needs an empty-array marker for
	// PHP, otherwise productsBox validation fires (see probe §2).
	products := deref(p.Products)
	if len(products) == 0 {
		form.Set("products[]", "")
	} else {
		for _, id := range products {
			form.Add("products[]", strconv.FormatInt(id, 10))
		}
	}
	return form
}

// mergeProjectBaseline copies baseline and overrides only the fields the
// caller explicitly set on input (non-nil pointers).
//
// `Multiple` is a deliberate exception: ZenTao only honours `multiple`
// on project-create, never on project-edit (per probe + owner guidance).
// Edit POSTs that pass a different `multiple` are silently ignored, so
// we always replay the baseline value — never the caller's override.
// At the TF layer, `multiple` carries RequiresReplace so user-initiated
// flips destroy+create rather than reach this merge with a changed value.
func mergeProjectBaseline(input, baseline *Project) *Project {
	out := *baseline
	if input.ID != nil {
		out.ID = input.ID
	}
	if input.Name != nil {
		out.Name = input.Name
	}
	if input.Model != nil {
		out.Model = input.Model
	}
	if input.Begin != nil {
		out.Begin = input.Begin
	}
	if input.End != nil {
		out.End = input.End
	}
	if input.Parent != nil {
		out.Parent = input.Parent
	}
	if input.Products != nil {
		out.Products = input.Products
	}
	if input.WorkflowGroup != nil {
		out.WorkflowGroup = input.WorkflowGroup
	}
	// Multiple: NEVER take input — see godoc above.
	if input.ACL != nil {
		out.ACL = input.ACL
	}
	if input.PM != nil {
		out.PM = input.PM
	}
	if input.PO != nil {
		out.PO = input.PO
	}
	if input.QD != nil {
		out.QD = input.QD
	}
	if input.RD != nil {
		out.RD = input.RD
	}
	if input.Desc != nil {
		out.Desc = input.Desc
	}
	if input.Deleted != nil {
		out.Deleted = input.Deleted
	}
	return &out
}

// GetProject reads via `/project-view-{id}.json`.
func (c *Client) GetProject(ctx context.Context, id int64) (*Project, error) {
	body, status, err := c.doController(ctx, "project", "view", []string{strconv.FormatInt(id, 10)}, nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var probe struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("decode get-project probe: %w (body=%s)", err, string(body))
	}
	if probe.Status == "" && len(probe.Data) == 0 {
		return nil, ErrNotFound
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode get-project outer: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner projectViewInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode get-project inner: %w (body=%s)", err, string(body))
	}
	if len(inner.Project) == 0 || string(inner.Project) == "null" || string(inner.Project) == "false" {
		return nil, ErrNotFound
	}
	var prod Project
	if err := json.Unmarshal(inner.Project, &prod); err != nil {
		return nil, fmt.Errorf("decode get-project wire: %w (body=%s)", err, string(body))
	}
	ids, err := int64SliceFromIDKeyedMap(inner.Products)
	if err != nil {
		return nil, fmt.Errorf("decode get-project products map: %w (body=%s)", err, string(body))
	}
	prod.Products = &ids
	prod.Name = decodeEntitiesPtr(prod.Name)
	return &prod, nil
}

// CreateProject posts a form to project-create.json. The response echoes
// the new id directly; the wrapper re-fetches via GetProject so callers
// receive the full row including server-derived fields.
func (c *Client) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	if p == nil {
		return nil, fmt.Errorf("CreateProject: project is nil")
	}
	if p.Name == nil {
		return nil, fmt.Errorf("CreateProject: name required")
	}
	if p.Begin == nil {
		return nil, fmt.Errorf("CreateProject: begin required")
	}
	if p.End == nil {
		return nil, fmt.Errorf("CreateProject: end required")
	}
	if p.Model == nil {
		return nil, fmt.Errorf("CreateProject: model required")
	}
	if p.WorkflowGroup == nil {
		return nil, fmt.Errorf("CreateProject: workflowGroup required")
	}
	if p.ACL == nil {
		return nil, fmt.Errorf("CreateProject: acl required")
	}
	if p.Products == nil || len(*p.Products) == 0 {
		return nil, fmt.Errorf("CreateProject: at least one product required")
	}
	body, status, err := c.doControllerForm(ctx, "project", "create", nil, nil, p.toForm())
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
		return nil, fmt.Errorf("decode create-project: %w (body=%s)", err, string(body))
	}
	if resp.Result != "success" {
		var simple CtrlSimpleResponse
		_ = json.Unmarshal(body, &simple)
		return nil, classifyCtrlSimple(status, simple, body)
	}
	id, _ := resp.ID.Int64()
	if id == 0 {
		return nil, fmt.Errorf("create project: empty id in response (body=%s)", string(body))
	}
	return c.GetProject(ctx, id)
}

// UpdateProject fetches the baseline, merges caller overrides on top
// (M-Z merge), then submits the full form.
func (c *Client) UpdateProject(ctx context.Context, p *Project) (*Project, error) {
	if p == nil {
		return nil, fmt.Errorf("UpdateProject: project is nil")
	}
	if p.ID == nil || *p.ID == 0 {
		return nil, fmt.Errorf("UpdateProject: missing id")
	}
	id := *p.ID
	baseline, err := c.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("UpdateProject: fetch baseline for merge: %w", err)
	}
	merged := mergeProjectBaseline(p, baseline)
	body, status, err := c.doControllerForm(ctx, "project", "edit", []string{strconv.FormatInt(id, 10)}, nil, merged.toForm())
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
		return nil, fmt.Errorf("decode update-project envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	return c.GetProject(ctx, id)
}

// DeleteProject is a destructive GET. Unlike program-delete (which
// requires a positional `-yes` suffix), the bare `project-delete-{id}.json`
// form is accepted directly. The server is generously idempotent: missing
// id and already-deleted both return `{result:success, closeModal:true,
// load:"..."}` — the wrapper passes these through.
func (c *Client) DeleteProject(ctx context.Context, id int64) error {
	body, status, err := c.doController(ctx, "project", "delete", []string{strconv.FormatInt(id, 10)}, nil, nil)
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
		return fmt.Errorf("decode delete-project envelope: %w (body=%s)", err, string(body))
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
