package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type groupEditInner struct {
	Group json.RawMessage `json:"group"`
}

type groupListInner struct {
	Groups []json.RawMessage `json:"groups"`
}

// Group represents a ZenTao permission group. Project=0 is a system
// (org-wide) group; Project>0 is a project-scoped group.
type Group struct {
	ID        *int64  `json:"id,omitempty"`
	Project   *int64  `json:"project,omitempty"`
	Name      *string `json:"name,omitempty"`
	Role      *string `json:"role,omitempty"`
	Desc      *string `json:"desc,omitempty"`
	Vision    *string `json:"vision,omitempty"`
	Developer *int64  `json:"developer,omitempty"`
}

// UnmarshalJSON decodes a group-edit / project-group wire payload into
// *Group. ZenTao's controller surface returns id / project / developer
// as a mix of JSON numbers and quoted-number strings; the json.Number
// locals tolerate both shapes. The `acl` column (NULL on this deployment)
// and `users` (joined from zt_usergroup, not a column on zt_group) are
// intentionally ignored — encoding/json silently drops unknown keys.
// Absent fields stay nil so callers can distinguish "wire omitted this
// column" from "wire said empty string".
func (g *Group) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID        json.Number `json:"id"`
		Project   json.Number `json:"project"`
		Developer json.Number `json:"developer"`
		Name      *string     `json:"name"`
		Role      *string     `json:"role"`
		Desc      *string     `json:"desc"`
		Vision    *string     `json:"vision"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != "" {
		v, err := jsonNumberToInt64(raw.ID, "id")
		if err != nil {
			return err
		}
		g.ID = &v
	}
	if raw.Project != "" {
		v, err := jsonNumberToInt64(raw.Project, "project")
		if err != nil {
			return err
		}
		g.Project = &v
	}
	if raw.Developer != "" {
		v, err := jsonNumberToInt64(raw.Developer, "developer")
		if err != nil {
			return err
		}
		g.Developer = &v
	}
	g.Name = raw.Name
	g.Role = raw.Role
	g.Desc = raw.Desc
	g.Vision = raw.Vision
	return nil
}

// toForm always emits every form.php writeable field, even when the
// value is empty/0. `vision` defaults to "rnd" when absent because
// ZenTao's group form treats an empty vision as invalid.
func (g *Group) toForm() url.Values {
	form := url.Values{}
	form.Set("project", strconv.FormatInt(deref(g.Project), 10))
	form.Set("name", deref(g.Name))
	form.Set("role", deref(g.Role))
	form.Set("desc", deref(g.Desc))
	vision := deref(g.Vision)
	if vision == "" {
		vision = "rnd"
	}
	form.Set("vision", vision)
	if dev := deref(g.Developer); dev != 0 {
		form.Set("developer", strconv.FormatInt(dev, 10))
	}
	return form
}

// mergeGroupBaseline copies baseline and overrides only the fields the
// caller explicitly set on input (non-nil pointers). A nil pointer reads
// as "preserve baseline". This is the M-Z merge that makes UpdateGroup
// safe against ZenTao's non-PATCH semantics.
func mergeGroupBaseline(input, baseline *Group) *Group {
	out := *baseline
	if input.ID != nil {
		out.ID = input.ID
	}
	if input.Project != nil {
		out.Project = input.Project
	}
	if input.Name != nil {
		out.Name = input.Name
	}
	if input.Role != nil {
		out.Role = input.Role
	}
	if input.Desc != nil {
		out.Desc = input.Desc
	}
	if input.Vision != nil {
		out.Vision = input.Vision
	}
	if input.Developer != nil {
		out.Developer = input.Developer
	}
	return &out
}

// GetGroup fetches a group via the group-edit-{id} GET endpoint.
// Missing rows — HTTP 404, an envelope-fail "does not exist" reason,
// or the `group:null` payload — all collapse to ErrNotFound.
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
	var g Group
	if err := json.Unmarshal(inner.Group, &g); err != nil {
		return nil, fmt.Errorf("decode get-group wire: %w (body=%s)", err, string(body))
	}
	return &g, nil
}

// CreateGroup posts a form to group-create.json. The create endpoint
// does NOT echo the new id back, so we discover it via a follow-up
// list query (project-group for project>0, group-browse for project=0)
// filtered by name. After id resolution the wrapper re-fetches via
// GetGroup so callers receive the full server-side row.
func (c *Client) CreateGroup(ctx context.Context, g *Group) (*Group, error) {
	if g == nil {
		return nil, fmt.Errorf("CreateGroup: group is nil")
	}
	if g.Project != nil && *g.Project < 0 {
		return nil, fmt.Errorf("CreateGroup: project must be >= 0 (got %d)", *g.Project)
	}
	if g.Name == nil || *g.Name == "" {
		return nil, fmt.Errorf("CreateGroup: name required")
	}
	body, status, err := c.doControllerForm(ctx, "group", "create", nil, nil, g.toForm())
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
	id, err := c.findGroupIDByName(ctx, deref(g.Project), deref(g.Name))
	if err != nil {
		return nil, fmt.Errorf("create-group succeeded but post-create lookup failed: %w", err)
	}
	return c.GetGroup(ctx, id)
}

// findGroupIDByName looks up the id of a newly-created group by name.
// project=0 lives in the system list (group-browse); project>0 lives
// in the per-project list (project-group). Project-scoped rows are
// not returned by group-browse on Max 8.x — verified by probe.
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
	for _, raw := range inner.Groups {
		var row Group
		if err := json.Unmarshal(raw, &row); err != nil {
			return 0, fmt.Errorf("decode list-group row: %w (body=%s)", err, string(body))
		}
		if row.Name != nil && *row.Name == name && row.ID != nil {
			return *row.ID, nil
		}
	}
	return 0, ErrNotFound
}

// UpdateGroup fetches the baseline, merges caller overrides on top
// (M-Z merge), then submits the full form. The server returns success
// even when the target id is missing — the post-edit GetGroup is the
// only signal, and ErrNotFound bubbles up unchanged.
func (c *Client) UpdateGroup(ctx context.Context, g *Group) (*Group, error) {
	if g == nil {
		return nil, fmt.Errorf("UpdateGroup: group is nil")
	}
	if g.ID == nil || *g.ID == 0 {
		return nil, fmt.Errorf("UpdateGroup: id required")
	}
	id := *g.ID
	baseline, err := c.GetGroup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("UpdateGroup: fetch baseline for merge: %w", err)
	}
	merged := mergeGroupBaseline(g, baseline)
	body, status, err := c.doControllerForm(ctx, "group", "edit", []string{strconv.FormatInt(id, 10)}, nil, merged.toForm())
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
	return c.GetGroup(ctx, id)
}

// DeleteGroup is a destructive GET; do NOT add ?confirm=yes (that's
// the server contract, unlike user-delete). 404 and "already deleted"
// envelopes both collapse to nil.
func (c *Client) DeleteGroup(ctx context.Context, id int64) error {
	body, status, err := c.doController(ctx, "group", "delete", []string{strconv.FormatInt(id, 10)}, nil, nil)
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
		return fmt.Errorf("decode delete-group envelope: %w (body=%s)", err, string(body))
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
