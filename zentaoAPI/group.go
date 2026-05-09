package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Group is a ZenTao permission group: a row in zt_group. Two flavours
// share one table and are distinguished only by the `project` column:
//
//   - project = 0   → system group (org-wide; e.g. the built-in "管理员"
//     row that ships with every install). System groups grant
//     org-level role permissions.
//   - project > 0   → project-scoped permission group (the in-project
//     RBAC bucket users typically think of as "project group").
//
// Controller plumbing lives in `module/group/control.php` for both
// flavours — the `module/project/control.php` action of the same name
// is just a per-project listing view, not its own CRUD module.
//
// See docs/superpowers/specs/probe-group-controller.md for the
// probe-verified wire shapes and quirks. The two most surprising
// quirks, which shape this wrapper:
//
//  1. POST /group-edit-<id>.json on a NON-existent id silently returns
//     `{result:success, message:"保存成功"}` — same envelope as a real
//     successful update. UpdateGroup re-reads after the POST and
//     surfaces ErrNotFound when the row is still gone.
//
//  2. GET /group-delete-<id>.json is destructive WITHOUT confirm=yes
//     (unlike user-delete). DeleteGroup is therefore a GET with no
//     body and no `confirm` query — that's the actual server contract,
//     not an oversight. Idempotent on missing rows.
//
// Sensitive fields aren't a concern here (no passwords or tokens), so
// the read/write asymmetry that user.go uses for password redaction
// isn't replicated.
type Group struct {
	// Identity & immutable post-Create.
	ID      int `json:"-"` // server-assigned; populated post-Create via list-and-filter
	Project int `json:"project"`

	// Mutable.
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
	Desc string `json:"desc,omitempty"`

	// Server-managed; not surfaced to TF in v1, kept here so the wrapper
	// can decode without losing data on round-trip and so callers who
	// DO care can override the validator-required `vision` default.
	Vision    string `json:"-"` // server validator requires this; we always send "rnd" if unset
	Developer int    `json:"-"` // ignored by server on update (probe-confirmed); leave 0 by default
}

// groupCtrlWire mirrors the JSON the `group` module returns for a
// group row. Fields tagged json.Number tolerate both native int and
// string-encoded forms because Controller endpoints flip between them
// across actions (probe-controller-auth.md addendum). The ACL field
// is RawMessage because the same field is `""` on the project-group
// list view but `[]` on the group-edit single view.
type groupCtrlWire struct {
	ID        json.Number     `json:"id"`
	Project   json.Number     `json:"project"`
	Name      string          `json:"name"`
	Role      string          `json:"role"`
	Desc      string          `json:"desc"`
	ACL       json.RawMessage `json:"acl"` // tolerates "" and []
	Developer json.Number     `json:"developer"`
	Vision    string          `json:"vision"`
}

func (w groupCtrlWire) toGroup() (*Group, error) {
	id, err := jsonNumberToInt(w.ID, "id")
	if err != nil {
		return nil, err
	}
	project, err := jsonNumberToInt(w.Project, "project")
	if err != nil {
		return nil, err
	}
	developer, err := jsonNumberToInt(w.Developer, "developer")
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

// Path helpers. The `group` module owns CRUD; the `project` module's
// `group` action is the per-project listing view used to discover
// post-Create ids when project>0. For project=0 the system-wide
// group-browse endpoint surfaces the new id.
const groupCreatePath = "group-create.json"

func groupEditPath(id int) string {
	return controllerPath("group", "edit", []string{strconv.Itoa(id)})
}

func groupDeletePath(id int) string {
	return controllerPath("group", "delete", []string{strconv.Itoa(id)})
}

// groupListByProjectPath: per-project listing (project=N>0).
func groupListByProjectPath(projectID int) string {
	return controllerPath("project", "group", []string{strconv.Itoa(projectID)})
}

// groupBrowsePath: system-wide listing (returns project=0 rows only on
// Max 8.x — verified by probe).
const groupBrowsePath = "group-browse.json"

// groupEditInner is the inner JSON object decoded out of the
// stringified Data field of `group-edit-<id>.json` GET. Only `group`
// is consumed; the surrounding form-context fields (title/pager) are
// for HTML rendering and ignored.
type groupEditInner struct {
	Group json.RawMessage `json:"group"`
}

// groupListInner is the inner JSON object decoded out of the
// stringified Data field of either `project-group-<projectID>.json`
// (project>0) or `group-browse.json` (project=0). Other surrounding
// fields are not consumed.
type groupListInner struct {
	Groups []groupCtrlWire `json:"groups"`
}

// GetGroup fetches a group by numeric id via the `group-edit-<id>.json`
// GET endpoint. We use edit-GET as the read primitive because
// `group-view` does NOT exist on Max 8.x (probe finding §1) — same
// pattern as GetUser.
//
// Returns ErrNotFound when:
//   - HTTP 404
//   - inner.group is absent, `null`, or `false` (the empty-marker
//     shape ZenTao Controllers use when no row matched at HTTP 200)
//   - the envelope status is not "success" AND the reason matches
//     isNotFoundReason (e.g. "Group does not exist.")
func (c *Client) GetGroup(ctx context.Context, id int) (*Group, error) {
	body, status, err := c.doController(ctx, "group", "edit", []string{strconv.Itoa(id)}, nil, nil)
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
		return nil, fmt.Errorf("decode get-group envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner groupEditInner
	if err := DecodeData(env, &inner); err != nil {
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

// groupToForm encodes a *Group into the form-urlencoded shape the
// system `group` module expects on POST. Required by the server
// validator: name, project, vision (defaults to "rnd" if the caller
// didn't pin it). Empty-string role and desc are sent verbatim so a
// probe rename + clear works in one call.
func groupToForm(g *Group) url.Values {
	form := url.Values{}
	form.Set("name", g.Name)
	form.Set("project", strconv.Itoa(g.Project))
	form.Set("role", g.Role)
	form.Set("desc", g.Desc)
	if g.Vision != "" {
		form.Set("vision", g.Vision)
	} else {
		form.Set("vision", "rnd")
	}
	if g.Developer != 0 {
		form.Set("developer", strconv.Itoa(g.Developer))
	}
	return form
}

// CreateGroup creates a group via `group-create.json` POST with form-
// urlencoded body. The Controller does NOT echo the new row's id —
// we discover it by listing and matching on name. The list endpoint
// depends on the group's flavour:
//
//   - project = 0 → list via `group-browse.json` (system groups only;
//     project-scoped rows are excluded from this view on Max 8.x).
//   - project > 0 → list via `project-group-<projectID>.json`.
//
// Name uniqueness within the appropriate scope is the assumed lookup
// invariant. If the post-create lookup yields no matching row, we
// surface a loud error rather than fabricate a synthetic id.
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
		out.Vision = "rnd" // mirror the server-side default we forced on the wire
	}
	return &out, nil
}

// findGroupIDByName lists the appropriate group view and returns the
// id of the first row whose name matches exactly. Used post-Create
// to resolve the new row's id (the create endpoint doesn't echo it).
//
// Routing by project flavour is the whole reason this helper exists:
// the system-wide `group-browse.json` does NOT include project-scoped
// rows, so we cannot use one endpoint for both flavours.
//
// Returns ErrNotFound when no match is found.
func (c *Client) findGroupIDByName(ctx context.Context, projectID int, name string) (int, error) {
	var module, method string
	var pathArgs []string
	if projectID == 0 {
		module, method = "group", "browse"
	} else {
		module, method = "project", "group"
		pathArgs = []string{strconv.Itoa(projectID)}
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
	var env CtrlEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("decode list-group envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return 0, classifyCtrlError(status, env, body)
	}
	var inner groupListInner
	if err := DecodeData(env, &inner); err != nil {
		return 0, fmt.Errorf("decode list-group data: %w (body=%s)", err, string(body))
	}
	for _, w := range inner.Groups {
		if w.Name == name {
			id, err := jsonNumberToInt(w.ID, "id")
			if err != nil {
				return 0, err
			}
			return id, nil
		}
	}
	return 0, ErrNotFound
}

// UpdateGroup edits a group via `group-edit-<id>.json` POST with
// form-urlencoded body. On success, re-fetches via GetGroup to surface
// the authoritative server state.
//
// CRITICAL probe finding (§0.2): the server returns the SAME
// `{result:success, message:"保存成功"}` envelope for both real
// updates AND silent no-ops on non-existent ids. The post-POST GET is
// not a courtesy — it's the only signal that the row actually exists.
// If the re-fetch returns ErrNotFound, that is the Update's return.
func (c *Client) UpdateGroup(ctx context.Context, g *Group) (*Group, error) {
	if g == nil {
		return nil, fmt.Errorf("UpdateGroup: group is nil")
	}
	if g.ID == 0 {
		return nil, fmt.Errorf("UpdateGroup: id required")
	}
	body, status, err := c.doControllerForm(ctx, "group", "edit", []string{strconv.Itoa(g.ID)}, nil, groupToForm(g))
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

// DeleteGroup removes a group via `GET /group-delete-<id>.json`. NO
// `confirm=yes` is required — the server treats this GET as the
// destructive action immediately. This matches probe finding §0.1; do
// NOT add a confirm query thinking it's missing.
//
// Idempotent on missing rows: HTTP 404 and `result:success` envelopes
// (the server emits the same success shape whether the row existed or
// not) both return nil. Other failures (e.g. "insufficient privs")
// surface as *APIError via classifyCtrlSimple.
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
