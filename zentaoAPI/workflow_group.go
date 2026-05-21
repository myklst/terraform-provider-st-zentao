package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// WorkflowGroup is one zt_workflowgroup row; the project resource's
// workflow_group input references it by id.
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

// Two catalogs share these controller method names, distinguished by the
// zt_workflowgroup.type column: product (产品流程) and project (项目流程).
const (
	workflowGroupProductMethod = "product"
	workflowGroupProjectMethod = "project"
)

// ListProductWorkflowGroups returns the 产品流程 catalog.
func (c *Client) ListProductWorkflowGroups(ctx context.Context) ([]*WorkflowGroup, error) {
	return c.listWorkflowGroups(ctx, workflowGroupProductMethod)
}

// ListProjectWorkflowGroups returns the 项目流程 catalog.
func (c *Client) ListProjectWorkflowGroups(ctx context.Context) ([]*WorkflowGroup, error) {
	return c.listWorkflowGroups(ctx, workflowGroupProjectMethod)
}

// listWorkflowGroups returns one catalog (selected by controller method),
// dropping soft-deleted rows and sorting by id. Pagination is unimplemented:
// it fails loudly when a catalog spills past one page (pager.pageTotal > 1)
// rather than silently truncating.
func (c *Client) listWorkflowGroups(ctx context.Context, method string) ([]*WorkflowGroup, error) {
	body, status, err := c.doController(ctx, "workflowgroup", method, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode workflowgroup-%s response: %w (body=%s)", method, err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner workflowGroupListInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode workflowgroup-%s data: %w (body=%s)", method, err, string(body))
	}
	if inner.Pager.PageTotal > 1 {
		return nil, fmt.Errorf("listWorkflowGroups(%s): workflow group count exceeds a single page (pageTotal=%d); paginated fetch is not implemented — please contact the maintainer", method, inner.Pager.PageTotal)
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
