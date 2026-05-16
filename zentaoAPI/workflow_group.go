package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

// WorkflowGroup represents one row from ZenTao's `zt_workflowgroup` table —
// a preset combination of (projectModel, projectType) that the project
// resource's `workflow_group` form input references by id. The default
// Max 8.x install ships two: id=2 (scrumproduct: scrum + product-typed)
// and id=3 (scrumproject: scrum + project-typed). Admins can extend the
// catalog at runtime.
type WorkflowGroup struct {
	ID           *int64  `json:"id,omitempty"`
	Code         *string `json:"code,omitempty"`         // e.g. "scrumproduct"
	Name         *string `json:"name,omitempty"`         // display name (i18n)
	ProjectModel *string `json:"projectModel,omitempty"` // scrum / waterfall / kanban / agileplus / ...
	ProjectType  *string `json:"projectType,omitempty"`  // product / project
	Vision       *string `json:"vision,omitempty"`       // rnd / ops / lite
	Deliverable  *string `json:"deliverable,omitempty"`
}

func (w *WorkflowGroup) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID           json.Number `json:"id"`
		Code         *string     `json:"code"`
		Name         *string     `json:"name"`
		ProjectModel *string     `json:"projectModel"`
		ProjectType  *string     `json:"projectType"`
		Vision       *string     `json:"vision"`
		Deliverable  *string     `json:"deliverable"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != "" {
		v, err := jsonNumberToInt64(raw.ID, "id")
		if err != nil {
			return err
		}
		w.ID = &v
	}
	w.Code = raw.Code
	w.Name = raw.Name
	w.ProjectModel = raw.ProjectModel
	w.ProjectType = raw.ProjectType
	w.Vision = raw.Vision
	w.Deliverable = raw.Deliverable
	return nil
}

type workflowGroupViewInner struct {
	Group json.RawMessage `json:"group"`
}

// GetWorkflowGroup fetches one workflow group by id via the
// workflowgroup-view-{id}.json endpoint. Missing-id (HTTP 404 or
// envelope-fail "does not exist") collapses to ErrNotFound.
func (c *Client) GetWorkflowGroup(ctx context.Context, id int64) (*WorkflowGroup, error) {
	body, status, err := c.doController(ctx, "workflowgroup", "view", []string{strconv.FormatInt(id, 10)}, nil, nil)
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
		return nil, fmt.Errorf("decode get-workflowgroup envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner workflowGroupViewInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode get-workflowgroup data: %w (body=%s)", err, string(body))
	}
	if len(inner.Group) == 0 || string(inner.Group) == "null" || string(inner.Group) == "false" {
		return nil, ErrNotFound
	}
	var wfg WorkflowGroup
	if err := json.Unmarshal(inner.Group, &wfg); err != nil {
		return nil, fmt.Errorf("decode get-workflowgroup wire: %w (body=%s)", err, string(body))
	}
	return &wfg, nil
}

// ListWorkflowGroupIDs enumerates all workflow group ids visible to the
// authenticated user. ZenTao does not expose a dedicated list endpoint
// (workflowgroup-browse → "module has no browse method"); the only
// reliable enumeration path is the `project-create.json` form's
// `workflowGroupPairs` map. We POST an empty form body — empty POST is
// interpreted as "render the form" rather than "try to create" and
// returns the catalog as part of the form scaffolding.
//
// Returns ids sorted ascending for deterministic ordering. Use
// GetWorkflowGroup per-id to fetch the full row.
func (c *Client) ListWorkflowGroupIDs(ctx context.Context) ([]int64, error) {
	body, status, err := c.doControllerForm(ctx, "project", "create", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode project-create form envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner struct {
		WorkflowGroupPairs map[string]string `json:"workflowGroupPairs"`
	}
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode project-create form data: %w (body=%s)", err, string(body))
	}
	out := make([]int64, 0, len(inner.WorkflowGroupPairs))
	for k := range inner.WorkflowGroupPairs {
		n, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("workflowGroupPairs: non-integer key %q: %w", k, err)
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// FindWorkflowGroup looks up the workflow group whose (projectModel,
// projectType) pair matches the caller's filters. Enumerates via
// ListWorkflowGroupIDs and then fetches each candidate via
// GetWorkflowGroup until a match is found.
//
// Returns ErrNotFound when no group matches. Returns an error when
// multiple groups match (ZenTao admins can create duplicates — surface
// the ambiguity rather than silently picking).
func (c *Client) FindWorkflowGroup(ctx context.Context, projectModel, projectType string) (*WorkflowGroup, error) {
	if projectModel == "" {
		return nil, fmt.Errorf("FindWorkflowGroup: projectModel required")
	}
	if projectType == "" {
		return nil, fmt.Errorf("FindWorkflowGroup: projectType required")
	}
	ids, err := c.ListWorkflowGroupIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("FindWorkflowGroup: list ids: %w", err)
	}
	var matches []*WorkflowGroup
	for _, id := range ids {
		wfg, err := c.GetWorkflowGroup(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("FindWorkflowGroup: fetch id=%d: %w", id, err)
		}
		if deref(wfg.ProjectModel) == projectModel && deref(wfg.ProjectType) == projectType {
			matches = append(matches, wfg)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("FindWorkflowGroup: no group matches projectModel=%q projectType=%q: %w",
			projectModel, projectType, ErrNotFound)
	case 1:
		return matches[0], nil
	default:
		ids := make([]int64, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, deref(m.ID))
		}
		return nil, fmt.Errorf("FindWorkflowGroup: %d groups match projectModel=%q projectType=%q (ids=%v) — admin has duplicates, please disambiguate",
			len(matches), projectModel, projectType, ids)
	}
}
