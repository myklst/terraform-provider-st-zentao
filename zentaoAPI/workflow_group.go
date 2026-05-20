package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// WorkflowGroup represents one row from ZenTao's `zt_workflowgroup` table —
// a preset combination of (projectModel, projectType) that the project
// resource's `workflow_group` form input references by id. The default
// Max 8.x install ships ten (scrum/waterfall/agileplus/waterfallplus/kanban
// × product/project). Admins can extend the catalog at runtime.
type WorkflowGroup struct {
	ID           *int64  `json:"id,omitempty"`
	Code         *string `json:"code,omitempty"`         // e.g. "scrumproduct"
	Name         *string `json:"name,omitempty"`         // display name (i18n)
	ProjectModel *string `json:"projectModel,omitempty"` // scrum / waterfall / kanban / agileplus / ...
	ProjectType  *string `json:"projectType,omitempty"`  // product / project
	Vision       *string `json:"vision,omitempty"`       // rnd / ops / lite
	Deliverable  *string `json:"deliverable,omitempty"`
	Deleted      *int64  `json:"deleted,omitempty"` // soft-delete flag; 1 == removed
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
		Deleted      json.Number `json:"deleted"`
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
	if raw.Deleted != "" {
		v, err := jsonNumberToInt64(raw.Deleted, "deleted")
		if err != nil {
			return err
		}
		w.Deleted = &v
	}
	w.Code = raw.Code
	w.Name = raw.Name
	w.ProjectModel = raw.ProjectModel
	w.ProjectType = raw.ProjectType
	w.Vision = raw.Vision
	w.Deliverable = raw.Deliverable
	return nil
}

type workflowGroupListInner struct {
	Groups map[string]WorkflowGroup `json:"groups"`
	Pager  struct {
		PageTotal int `json:"pageTotal"`
	} `json:"pager"`
}

// ListWorkflowGroups enumerates every workflow group visible to the
// authenticated user via the workflowgroup-project.json controller route.
// A single call returns the full catalog (both product-typed and
// project-typed rows) keyed by id; see
// docs/superpowers/specs/probe-workflowgroup-project.md.
//
// Soft-deleted rows (deleted == 1) are dropped. Results are sorted by id
// ascending for deterministic ordering.
//
// Pagination is not implemented: the default page holds 20 rows and the
// factory catalog is 10, so realistic catalogs fit one page. If an admin
// grows the catalog past one page (pager.pageTotal > 1) we fail loudly
// rather than silently truncate — the paging parameter format has not been
// probed, so completing it is deferred until someone actually hits it.
func (c *Client) ListWorkflowGroups(ctx context.Context) ([]*WorkflowGroup, error) {
	body, status, err := c.doController(ctx, "workflowgroup", "project", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode list-workflowgroup envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner workflowGroupListInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode list-workflowgroup data: %w (body=%s)", err, string(body))
	}
	if inner.Pager.PageTotal > 1 {
		return nil, fmt.Errorf("ListWorkflowGroups: workflow group count exceeds a single page (pageTotal=%d); paginated fetch is not implemented — please contact the maintainer", inner.Pager.PageTotal)
	}
	out := make([]*WorkflowGroup, 0, len(inner.Groups))
	for _, g := range inner.Groups {
		if deref(g.Deleted) == 1 {
			continue
		}
		wfg := g
		out = append(out, &wfg)
	}
	sort.Slice(out, func(i, j int) bool { return deref(out[i].ID) < deref(out[j].ID) })
	return out, nil
}

// FindWorkflowGroup looks up the workflow group whose (projectModel,
// projectType) pair matches the caller's filters by enumerating via
// ListWorkflowGroups and filtering in memory.
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
	groups, err := c.ListWorkflowGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("FindWorkflowGroup: list groups: %w", err)
	}
	var matches []*WorkflowGroup
	for _, g := range groups {
		if deref(g.ProjectModel) == projectModel && deref(g.ProjectType) == projectType {
			matches = append(matches, g)
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
