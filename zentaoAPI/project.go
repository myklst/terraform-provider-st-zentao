package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// Project is the canonical in-memory representation of a ZenTao project
// (an actual project row, type=project — not a sprint and not a program).
//
// ZenTao's `zt_project` table multiplexes projects, sprints, and programs
// behind a single REST surface; this struct intentionally locks the wire
// `type` to "project". Same convention as Product/Program: fields without
// `json:"-"` go on the wire for POST/PUT; fields with `json:"-"` are
// decoded from GET responses but never echoed back to the server.
//
// Beyond the publicly-documented fields, ZenTao Max 8.x requires two
// extra body fields on POST that are not in the V2 docs:
//
//   - Products      (>= 1 product id) — validator key in errors is
//     "productsBox"; on the wire the key is "products".
//   - WorkflowGroup (any integer) — controls the project workflow scheme.
//
// See docs/superpowers/specs/probe-project-v2.md for the full surface.
type Project struct {
	// Identity & content (writeable; sent on POST/PUT).
	ID            int    `json:"id,omitempty"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	Type          string `json:"type"` // always "project" for this resource
	Begin         string `json:"begin,omitempty"`
	End           string `json:"end,omitempty"`
	Parent        int    `json:"parent"` // parent program id (0 = no parent)
	Products      []int  `json:"products,omitempty"`
	WorkflowGroup int    `json:"workflowGroup,omitempty"`

	// Optional writeable per probe-project-v2.md.
	ACL         string `json:"acl,omitempty"`
	PM          string `json:"PM,omitempty"`
	PO          string `json:"PO,omitempty"`
	QD          string `json:"QD,omitempty"`
	RD          string `json:"RD,omitempty"`
	Description string `json:"desc,omitempty"`

	// Read-only / server-managed (decoded from GET, never sent on write).
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

// projectV2Wire mirrors the GET-shape on the wire. Numeric/FK columns
// arrive as JSON strings ("0", "95"), so each one is decoded via
// json.Number for forgiving unmarshalling (same pattern as productV2Wire).
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
	Status        string      `json:"status"`
	ACL           string      `json:"acl"`
	PM            string      `json:"PM"`
	PO            string      `json:"PO"`
	QD            string      `json:"QD"`
	RD            string      `json:"RD"`
	Description   string      `json:"desc"`
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
	id, err := jsonNumberToInt(w.ID, "id")
	if err != nil {
		return nil, err
	}
	parent, err := jsonNumberToInt(w.Parent, "parent")
	if err != nil {
		return nil, err
	}
	wfg, err := jsonNumberToInt(w.WorkflowGroup, "workflowGroup")
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
		Status:        w.Status,
		ACL:           w.ACL,
		PM:            w.PM,
		PO:            w.PO,
		QD:            w.QD,
		RD:            w.RD,
		Description:   w.Description,
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

func projectPath(id int) string {
	return projectsPath + "/" + strconv.Itoa(id)
}

const projectsPath = apiV2PathPrefix + "projects"

// GetProject fetches a project by ID via GET /api.php/v2/projects/{id}.
// Returns ErrNotFound on HTTP 404 OR on the {"status":"fail",
// "message":"Project does not exist."} shape ZenTao v2 emits at HTTP
// 200 instead of a real 404 (verified in probe-project-v2.md §4).
//
// Also returns ErrNotFound when the row's `type` is not "project" —
// the same `zt_project` table holds sprints and programs distinguished
// by the type discriminator, and this resource intentionally manages
// only project rows. Returning ErrNotFound makes Terraform `Read`
// remove the resource from state, mirroring "the row I owned is gone".
func (c *Client) GetProject(ctx context.Context, id int) (*Project, error) {
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
	if out.Type != "project" {
		// Defensive: this resource manages only type=project rows.
		// A row that became a sprint/program via an out-of-band edit
		// is treated as deleted from Terraform's perspective.
		return nil, ErrNotFound
	}
	return out, nil
}

// CreateProject creates a project via POST /api.php/v2/projects. v2 only
// echoes back the new id; the caller should re-fetch via GetProject if
// it needs server-defaulted/derived fields (vision, hasProduct, etc.).
//
// The wire body must always send `type:"project"` because ZenTao's
// `zt_project` table is shared with sprint/program rows; this method
// will overwrite any caller-supplied Type to keep the invariant.
func (c *Client) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	send := *p // shallow copy so we don't mutate caller's struct
	send.Type = "project"
	body, status, err := c.doV2Request(ctx, http.MethodPost, projectsPath, nil, &send)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	// ZenTao validation failures sometimes use {"status":"fail",...} and
	// sometimes {"result":"fail",...} — the per-field message map is the
	// same shape but the discriminator key differs. Decode both before
	// asserting success.
	//
	// Avoid embedding ZentaoResponse here because the validation envelope
	// puts a *map* under "message" (e.g. {"productsBox":"..."}) instead of
	// a string, and ZentaoResponse.Message is typed as string. apiError
	// does a best-effort decode that absorbs the type mismatch, so we
	// only decode the discriminator + id strictly.
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
	out.ID = int(id)
	out.Type = "project"
	return &out, nil
}

// UpdateProject edits a project via PUT /api.php/v2/projects/{id}. v2
// only returns {status}, so on success we re-fetch the project to
// surface authoritative state (server-set fields like path, percent,
// lastEditedDate). ErrNotFound is returned for both HTTP 404 and the
// HTTP 200 + "does not exist" envelope.
//
// PUT is more permissive than POST: begin/end may be omitted and prior
// values are retained; products[] cannot be set to empty though.
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
	// Same Status/Result discriminator dance as CreateProject; the
	// not-found "Project does not exist." string shape uses Message as
	// a plain string so we can also pull the reason here.
	var resp struct {
		Status  string `json:"status"`
		Result  string `json:"result"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		// Validation failures with object-shaped message would land
		// here; fall back to apiError which absorbs the malformed
		// shape via best-effort decoding.
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

// DeleteProject removes a project via DELETE /api.php/v2/projects/{id}.
// Idempotent on missing rows: ZenTao v2 returns HTTP 200 + {"status":
// "fail","message":"Project does not exist."} for both already-deleted
// and never-existed ids (probe-project-v2.md §4). We treat HTTP 404
// the same way for forward-compatibility with future ZenTao versions
// that might tighten the response.
func (c *Client) DeleteProject(ctx context.Context, id int) error {
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
