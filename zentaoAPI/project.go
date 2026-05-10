package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// Project represents a ZenTao project (type=project; not sprint, not program).
type Project struct {
	ID            int64   `json:"id,omitempty"`
	Name          string  `json:"name"`
	Model         string  `json:"model"`
	Type          string  `json:"type"`
	Begin         string  `json:"begin,omitempty"`
	End           string  `json:"end,omitempty"`
	Parent        int64   `json:"parent"`
	Products      []int64 `json:"products,omitempty"`
	WorkflowGroup int64   `json:"workflowGroup,omitempty"`
	Multiple      string  `json:"multiple,omitempty"`

	ACL  string `json:"acl,omitempty"`
	PM   string `json:"PM,omitempty"`
	PO   string `json:"PO,omitempty"`
	QD   string `json:"QD,omitempty"`
	RD   string `json:"RD,omitempty"`
	Desc string `json:"desc,omitempty"`

	Code         string `json:"-"`
	Status       string `json:"-"`
	Lifetime     string `json:"-"`
	OpenedBy     string `json:"-"`
	OpenedDate   string `json:"-"`
	LastEditedBy string `json:"-"`
	RealBegan    string `json:"-"`
	RealEnd      string `json:"-"`
	Progress     string `json:"-"`
	TeamCount    string `json:"-"`
	Budget       string `json:"-"`
	BudgetUnit   string `json:"-"`
}

type projectV2Wire struct {
	ID            json.Number `json:"id"`
	Name          string      `json:"name"`
	Code          string      `json:"code"`
	Model         string      `json:"model"`
	Type          string      `json:"type"`
	Begin         string      `json:"begin"`
	End           string      `json:"end"`
	Parent        json.Number `json:"parent"`
	WorkflowGroup json.Number `json:"workflowGroup"`
	Multiple      string      `json:"multiple"`
	Status        string      `json:"status"`
	ACL           string      `json:"acl"`
	PM            string      `json:"PM"`
	PO            string      `json:"PO"`
	QD            string      `json:"QD"`
	RD            string      `json:"RD"`
	Desc          string      `json:"desc"`
	Lifetime      string      `json:"lifetime"`
	OpenedBy      string      `json:"openedBy"`
	OpenedDate    string      `json:"openedDate"`
	LastEditedBy  string      `json:"lastEditedBy"`
	RealBegan     string      `json:"realBegan"`
	RealEnd       string      `json:"realEnd"`
	Progress      string      `json:"progress"`
	TeamCount     string      `json:"teamCount"`
	Budget        string      `json:"budget"`
	BudgetUnit    string      `json:"budgetUnit"`
}

func (w projectV2Wire) toProject() (*Project, error) {
	id, err := jsonNumberToInt64(w.ID, "id")
	if err != nil {
		return nil, err
	}
	parent, err := jsonNumberToInt64(w.Parent, "parent")
	if err != nil {
		return nil, err
	}
	wfg, err := jsonNumberToInt64(w.WorkflowGroup, "workflowGroup")
	if err != nil {
		return nil, err
	}
	return &Project{
		ID:            id,
		Name:          w.Name,
		Code:          w.Code,
		Model:         w.Model,
		Type:          w.Type,
		Begin:         w.Begin,
		End:           w.End,
		Parent:        parent,
		WorkflowGroup: wfg,
		Multiple:      w.Multiple,
		Status:        w.Status,
		ACL:           w.ACL,
		PM:            w.PM,
		PO:            w.PO,
		QD:            w.QD,
		RD:            w.RD,
		Desc:          w.Desc,
		Lifetime:      w.Lifetime,
		OpenedBy:      w.OpenedBy,
		OpenedDate:    w.OpenedDate,
		LastEditedBy:  w.LastEditedBy,
		RealBegan:     w.RealBegan,
		RealEnd:       w.RealEnd,
		Progress:      w.Progress,
		TeamCount:     w.TeamCount,
		Budget:        w.Budget,
		BudgetUnit:    w.BudgetUnit,
	}, nil
}

func projectPath(id int64) string {
	return projectsPath + "/" + strconv.FormatInt(id, 10)
}

const projectsPath = apiV2PathPrefix + "projects"

func (c *Client) GetProject(ctx context.Context, id int64) (*Project, error) {
	body, status, err := c.doV2Request(ctx, http.MethodGet, projectPath(id), nil, nil)
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
		Project projectV2Wire `json:"project"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode get-project: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" {
		if isNotFoundReason(resp.ZentaoFailReason()) {
			return nil, ErrNotFound
		}
		return nil, apiError(status, body)
	}
	out, err := resp.Project.toProject()
	if err != nil {
		return nil, err
	}
	// Sprint/program rows live in the same table; treat as gone.
	if out.Type != "project" {
		return nil, ErrNotFound
	}
	return out, nil
}

func (c *Client) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	send := *p
	send.Type = "project"
	body, status, err := c.doV2Request(ctx, http.MethodPost, projectsPath, nil, &send)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	// Validation failures use either {"status":"fail",...} or {"result":"fail",...}.
	var resp struct {
		Status string      `json:"status"`
		Result string      `json:"result"`
		ID     json.Number `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-project: %w (body=%s)", err, string(body))
	}
	if resp.Status != "success" || resp.Result == "fail" {
		return nil, apiError(status, body)
	}
	id, _ := resp.ID.Int64()
	if id == 0 {
		return nil, fmt.Errorf("create project: empty id in response (body=%s)", string(body))
	}
	out := *p
	out.ID = id
	out.Type = "project"
	return &out, nil
}

func (c *Client) UpdateProject(ctx context.Context, p *Project) (*Project, error) {
	if p.ID == 0 {
		return nil, fmt.Errorf("UpdateProject: missing id")
	}
	send := *p
	send.Type = "project"
	body, status, err := c.doV2Request(ctx, http.MethodPut, projectPath(p.ID), nil, &send)
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
		Status  string `json:"status"`
		Result  string `json:"result"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, apiError(status, body)
	}
	if resp.Status != "success" || resp.Result == "fail" {
		if isNotFoundReason(resp.Message) {
			return nil, ErrNotFound
		}
		return nil, apiError(status, body)
	}
	return c.GetProject(ctx, p.ID)
}

func (c *Client) DeleteProject(ctx context.Context, id int64) error {
	body, status, err := c.doV2Request(ctx, http.MethodDelete, projectPath(id), nil, nil)
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
		return fmt.Errorf("decode delete-project: %w (body=%s)", err, string(body))
	}
	if resp.Status == "success" {
		return nil
	}
	if isNotFoundReason(resp.ZentaoFailReason()) {
		return nil
	}
	return apiError(status, body)
}
