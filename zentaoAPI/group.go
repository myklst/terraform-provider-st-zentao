package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Group represents a ZenTao permission group. project=0 is a system
// group; project>0 is a project-scoped group.
type Group struct {
	ID      int64 `json:"-"`
	Project int64 `json:"project"`

	Name string `json:"name"`
	Role string `json:"role,omitempty"`
	Desc string `json:"desc,omitempty"`

	Vision    string `json:"-"`
	Developer int64  `json:"-"`
}

type groupCtrlWire struct {
	ID        json.Number     `json:"id"`
	Project   json.Number     `json:"project"`
	Name      string          `json:"name"`
	Role      string          `json:"role"`
	Desc      string          `json:"desc"`
	ACL       json.RawMessage `json:"acl"`
	Developer json.Number     `json:"developer"`
	Vision    string          `json:"vision"`
}

func (w groupCtrlWire) toGroup() (*Group, error) {
	id, err := jsonNumberToInt64(w.ID, "id")
	if err != nil {
		return nil, err
	}
	project, err := jsonNumberToInt64(w.Project, "project")
	if err != nil {
		return nil, err
	}
	developer, err := jsonNumberToInt64(w.Developer, "developer")
	if err != nil {
		return nil, err
	}
	return &Group{
		ID:        id,
		Project:   project,
		Name:      w.Name,
		Role:      w.Role,
		Desc:      w.Desc,
		Vision:    w.Vision,
		Developer: developer,
	}, nil
}

const groupCreatePath = "group-create.json"

func groupEditPath(id int) string {
	return controllerPath("group", "edit", []string{strconv.Itoa(id)})
}

func groupDeletePath(id int) string {
	return controllerPath("group", "delete", []string{strconv.Itoa(id)})
}

func groupListByProjectPath(projectID int) string {
	return controllerPath("project", "group", []string{strconv.Itoa(projectID)})
}

const groupBrowsePath = "group-browse.json"

type groupEditInner struct {
	Group json.RawMessage `json:"group"`
}

type groupListInner struct {
	Groups []groupCtrlWire `json:"groups"`
}

func (c *Client) GetGroup(ctx context.Context, id int64) (*Group, error) {
	body, status, err := c.doController(ctx, "group", "edit", []string{strconv.FormatInt(id, 10)}, nil, nil)
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
		return nil, fmt.Errorf("decode get-group envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner groupEditInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode get-group data: %w (body=%s)", err, string(body))
	}
	if len(inner.Group) == 0 || string(inner.Group) == "null" || string(inner.Group) == "false" {
		return nil, ErrNotFound
	}
	var wire groupCtrlWire
	if err := json.Unmarshal(inner.Group, &wire); err != nil {
		return nil, fmt.Errorf("decode get-group wire: %w (body=%s)", err, string(body))
	}
	return wire.toGroup()
}

func groupToForm(g *Group) url.Values {
	form := url.Values{}
	form.Set("name", g.Name)
	form.Set("project", strconv.FormatInt(g.Project, 10))
	form.Set("role", g.Role)
	form.Set("desc", g.Desc)
	if g.Vision != "" {
		form.Set("vision", g.Vision)
	} else {
		form.Set("vision", "rnd")
	}
	if g.Developer != 0 {
		form.Set("developer", strconv.FormatInt(g.Developer, 10))
	}
	return form
}

// CreateGroup discovers the new id via list-and-filter on name because
// the create endpoint does not echo it.
func (c *Client) CreateGroup(ctx context.Context, g *Group) (*Group, error) {
	if g == nil {
		return nil, fmt.Errorf("CreateGroup: group is nil")
	}
	if g.Project < 0 {
		return nil, fmt.Errorf("CreateGroup: project must be >= 0 (got %d)", g.Project)
	}
	if g.Name == "" {
		return nil, fmt.Errorf("CreateGroup: name required")
	}

	body, status, err := c.doControllerForm(ctx, "group", "create", nil, nil, groupToForm(g))
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-group envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}

	id, err := c.findGroupIDByName(ctx, g.Project, g.Name)
	if err != nil {
		return nil, fmt.Errorf("create-group succeeded but post-create lookup failed: %w", err)
	}
	out := *g
	out.ID = id
	if out.Vision == "" {
		out.Vision = "rnd"
	}
	return &out, nil
}

func (c *Client) findGroupIDByName(ctx context.Context, projectID int64, name string) (int64, error) {
	var module, method string
	var pathArgs []string
	if projectID == 0 {
		module, method = "group", "browse"
	} else {
		module, method = "project", "group"
		pathArgs = []string{strconv.FormatInt(projectID, 10)}
	}
	body, status, err := c.doController(ctx, module, method, pathArgs, nil, nil)
	if err != nil {
		return 0, err
	}
	if status == http.StatusNotFound {
		return 0, ErrNotFound
	}
	if status >= 400 {
		return 0, apiError(status, body)
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("decode list-group envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return 0, classifyCtrlError(status, env, body)
	}
	var inner groupListInner
	if err := env.DecodeData(&inner); err != nil {
		return 0, fmt.Errorf("decode list-group data: %w (body=%s)", err, string(body))
	}
	for _, w := range inner.Groups {
		if w.Name == name {
			id, err := jsonNumberToInt64(w.ID, "id")
			if err != nil {
				return 0, err
			}
			return id, nil
		}
	}
	return 0, ErrNotFound
}

// UpdateGroup re-fetches after POST because the server returns success
// even when the target id is missing — the GET is the only signal.
func (c *Client) UpdateGroup(ctx context.Context, g *Group) (*Group, error) {
	if g == nil {
		return nil, fmt.Errorf("UpdateGroup: group is nil")
	}
	if g.ID == 0 {
		return nil, fmt.Errorf("UpdateGroup: id required")
	}
	body, status, err := c.doControllerForm(ctx, "group", "edit", []string{strconv.FormatInt(g.ID, 10)}, nil, groupToForm(g))
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
		return nil, fmt.Errorf("decode update-group envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	return c.GetGroup(ctx, g.ID)
}

// DeleteGroup is a destructive GET; do NOT add ?confirm=yes (that's the
// server contract, unlike user-delete).
func (c *Client) DeleteGroup(ctx context.Context, id int) error {
	body, status, err := c.doController(ctx, "group", "delete", []string{strconv.Itoa(id)}, nil, nil)
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
	if err := json.Unmarshal(body, &resp); err == nil && resp.Result != "" {
		if resp.IsSuccess() {
			return nil
		}
		return classifyCtrlSimple(status, resp, body)
	}
	return apiError(status, body)
}

// Path helpers are referenced by future call sites and integration
// tests; keep the unused-warning quiet today.
var _ = groupCreatePath
var _ = groupEditPath
var _ = groupDeletePath
var _ = groupListByProjectPath
var _ = groupBrowsePath
