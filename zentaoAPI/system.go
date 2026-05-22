package zentaoapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type systemEditInner struct {
	System json.RawMessage `json:"system"`
}

type systemGetByNameInner struct {
	AppInfo json.RawMessage `json:"appInfo"`
}

// System represents a ZenTao application (the `system` module's
// application entity, not the DevOps system-admin surface). It belongs
// to a Product (set as the create-route arg, immutable thereafter).
type System struct {
	ID          *int64  `json:"id,omitempty"`
	Product     *int64  `json:"product,omitempty"`
	Name        *string `json:"name,omitempty"`
	Desc        *string `json:"desc,omitempty"`
	Integrated  *int64  `json:"integrated,omitempty"`
	Children    *string `json:"children,omitempty"`
	Status      *string `json:"status,omitempty"`
	CreatedBy   *string `json:"createdBy,omitempty"`
	CreatedDate *string `json:"createdDate,omitempty"`
	EditedBy    *string `json:"editedBy,omitempty"`
	EditedDate  *string `json:"editedDate,omitempty"`
	Deleted     *bool   `json:"deleted,omitempty"`
}

// UnmarshalJSON decodes a system row (edit-GET or showAll) into *System.
// id / product / integrated / deleted arrive as a mix of JSON numbers and
// quoted-number strings; json.Number locals tolerate both. children is a
// comma-separated id string on the parent row. Absent fields stay nil so
// callers can distinguish "wire omitted" from "wire said empty".
func (s *System) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID          json.Number `json:"id"`
		Product     json.Number `json:"product"`
		Integrated  json.Number `json:"integrated"`
		Children    *string     `json:"children"`
		Name        *string     `json:"name"`
		Desc        *string     `json:"desc"`
		Status      *string     `json:"status"`
		CreatedBy   *string     `json:"createdBy"`
		CreatedDate *string     `json:"createdDate"`
		EditedBy    *string     `json:"editedBy"`
		EditedDate  *string     `json:"editedDate"`
		Deleted     json.Number `json:"deleted"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != "" {
		v, err := jsonNumberToInt64(raw.ID, "id")
		if err != nil {
			return err
		}
		s.ID = &v
	}
	if raw.Product != "" {
		v, err := jsonNumberToInt64(raw.Product, "product")
		if err != nil {
			return err
		}
		s.Product = &v
	}
	if raw.Integrated != "" {
		v, err := jsonNumberToInt64(raw.Integrated, "integrated")
		if err != nil {
			return err
		}
		s.Integrated = &v
	}
	if raw.Deleted != "" {
		d := jsonNumberToBool(raw.Deleted)
		s.Deleted = &d
	}
	s.Children = raw.Children
	s.Name = raw.Name
	s.Desc = raw.Desc
	s.Status = raw.Status
	s.CreatedBy = raw.CreatedBy
	s.CreatedDate = raw.CreatedDate
	s.EditedBy = raw.EditedBy
	s.EditedDate = raw.EditedDate
	return nil
}

// toForm emits the edit/create writeable fields. children is a comma
// string baseline expanded into repeated children[] keys — the array
// wire shape ZenTao's form expects. integrated and status are not
// edit-form keys (server-owned / toggled via dedicated endpoints).
func (s *System) toForm() url.Values {
	form := url.Values{}
	form.Set("name", deref(s.Name))
	form.Set("desc", deref(s.Desc))
	for _, id := range splitChildren(deref(s.Children)) {
		form.Add("children[]", id)
	}
	return form
}

// splitChildren turns a comma-separated id string ("686,687") into its
// trimmed, non-empty segments. An empty string yields no segments.
func splitChildren(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// mergeSystemBaseline copies baseline and overrides only the fields the
// caller explicitly set (non-nil pointers). A nil pointer reads as
// "preserve baseline" — the M-Z merge that keeps UpdateSystem safe against
// ZenTao's non-PATCH edit semantics (notably children, owned by the
// attachment resource).
func mergeSystemBaseline(input, baseline *System) *System {
	out := *baseline
	if input.ID != nil {
		out.ID = input.ID
	}
	if input.Name != nil {
		out.Name = input.Name
	}
	if input.Desc != nil {
		out.Desc = input.Desc
	}
	if input.Children != nil {
		out.Children = input.Children
	}
	return &out
}

// GetSystem fetches an application via system-edit-{id} GET. Missing rows
// (HTTP 404, `system:false`, or a soft-deleted `deleted=1` tombstone)
// all collapse to ErrNotFound — ZenTao has no hard delete, and showAll
// still lists tombstones.
func (c *Client) GetSystem(ctx context.Context, id int64) (*System, error) {
	body, status, err := c.doController(ctx, "system", "edit", []string{strconv.FormatInt(id, 10)}, nil, nil)
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
		return nil, fmt.Errorf("decode get-system response: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner systemEditInner
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode get-system data: %w (body=%s)", err, string(body))
	}
	if len(inner.System) == 0 || string(inner.System) == "false" || string(inner.System) == "null" {
		return nil, ErrNotFound
	}
	var s System
	if err := json.Unmarshal(inner.System, &s); err != nil {
		return nil, fmt.Errorf("decode get-system wire: %w (body=%s)", err, string(body))
	}
	if deref(s.Deleted) {
		return nil, ErrNotFound
	}
	return &s, nil
}

// CreateSystem posts to system-create-{productID} (productID is a URL arg,
// not a form key). The create response carries no id, so the new id is
// discovered via system-showAll filtered by name+product, then the full
// row is refetched.
func (c *Client) CreateSystem(ctx context.Context, s *System) (*System, error) {
	if s == nil {
		return nil, fmt.Errorf("CreateSystem: system is nil")
	}
	if s.Name == nil || *s.Name == "" {
		return nil, fmt.Errorf("CreateSystem: name required")
	}
	if s.Product == nil || *s.Product <= 0 {
		return nil, fmt.Errorf("CreateSystem: product required (positive id)")
	}
	productID := *s.Product
	body, status, err := c.doControllerForm(ctx, "system", "create", []string{strconv.FormatInt(productID, 10)}, nil, s.toForm())
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode create-system response: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	id, err := c.findSystemIDByName(ctx, productID, *s.Name)
	if err != nil {
		return nil, fmt.Errorf("create-system succeeded but post-create lookup failed: %w", err)
	}
	// Status reconciliation: if the caller wants a non-default status,
	// toggle it after create (status is not a create-form key).
	if s.Status != nil && *s.Status != "" && *s.Status != "active" {
		if err := c.SetSystemStatus(ctx, id, *s.Status); err != nil {
			return nil, fmt.Errorf("create-system set status: %w", err)
		}
	}
	return c.GetSystem(ctx, id)
}

// findSystemIDByName resolves an application's id via the
// system-getbyname-{hex(name)} endpoint. The name is hex-encoded because a
// ZenTao PATH_INFO arg must avoid both `/` (URL path separator) and `-`
// (ZenTao's segment separator); hex's `0-9a-f` alphabet sidesteps both.
// Application names are unique across live and soft-deleted rows (create
// rejects duplicates), so the lookup is unambiguous. The endpoint matches
// by name only and does not filter tombstones, so a soft-deleted hit
// (`deleted=1`) or a `false` payload both mean "no live row" -> ErrNotFound;
// the product is verified defensively.
func (c *Client) findSystemIDByName(ctx context.Context, productID int64, name string) (int64, error) {
	arg := hex.EncodeToString([]byte(name))
	body, status, err := c.doController(ctx, "system", "getbyname", []string{arg}, nil, nil)
	if err != nil {
		return 0, err
	}
	if status >= 400 {
		return 0, apiError(status, body)
	}
	var env CtrlResp
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("decode getbyname envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return 0, classifyCtrlError(status, env, body)
	}
	var inner systemGetByNameInner
	if err := env.DecodeData(&inner); err != nil {
		return 0, fmt.Errorf("decode getbyname data: %w (body=%s)", err, string(body))
	}
	if len(inner.AppInfo) == 0 || string(inner.AppInfo) == "false" || string(inner.AppInfo) == "null" {
		return 0, ErrNotFound
	}
	var row System
	if err := json.Unmarshal(inner.AppInfo, &row); err != nil {
		return 0, fmt.Errorf("decode getbyname appInfo: %w (body=%s)", err, string(body))
	}
	if deref(row.Deleted) {
		return 0, ErrNotFound
	}
	if productID > 0 && deref(row.Product) != productID {
		return 0, fmt.Errorf("getbyname: %q resolved to id %d under product %d, want product %d: %w",
			name, deref(row.ID), deref(row.Product), productID, ErrNotFound)
	}
	if deref(row.ID) == 0 {
		return 0, ErrNotFound
	}
	return deref(row.ID), nil
}

// UpdateSystem M-Z-merges caller overrides onto the baseline, submits the
// edit form (name / desc / children), then reconciles status via the
// dedicated active/inactive endpoint when it changed. product is not an
// edit-form key, so it is never mutated here.
func (c *Client) UpdateSystem(ctx context.Context, s *System) (*System, error) {
	if s == nil {
		return nil, fmt.Errorf("UpdateSystem: system is nil")
	}
	if s.ID == nil || *s.ID == 0 {
		return nil, fmt.Errorf("UpdateSystem: id required")
	}
	id := *s.ID
	baseline, err := c.GetSystem(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("UpdateSystem: fetch baseline for merge: %w", err)
	}
	merged := mergeSystemBaseline(s, baseline)
	body, status, err := c.doControllerForm(ctx, "system", "edit", []string{strconv.FormatInt(id, 10)}, nil, merged.toForm())
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
		return nil, fmt.Errorf("decode update-system response: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return nil, classifyCtrlSimple(status, resp, body)
	}
	if s.Status != nil && *s.Status != deref(baseline.Status) {
		if err := c.SetSystemStatus(ctx, id, *s.Status); err != nil {
			return nil, fmt.Errorf("UpdateSystem set status: %w", err)
		}
	}
	return c.GetSystem(ctx, id)
}

// SetSystemStatus toggles an application's enabled state via the
// dedicated system-active / system-inactive endpoints (status is not an
// edit-form key).
func (c *Client) SetSystemStatus(ctx context.Context, id int64, status string) error {
	var method string
	switch status {
	case "active":
		method = "active"
	case "inactive":
		method = "inactive"
	default:
		return fmt.Errorf("SetSystemStatus: unsupported status %q (want active|inactive)", status)
	}
	body, httpStatus, err := c.doControllerForm(ctx, "system", method, []string{strconv.FormatInt(id, 10)}, nil, url.Values{})
	if err != nil {
		return err
	}
	if httpStatus == http.StatusNotFound {
		return ErrNotFound
	}
	if httpStatus >= 400 {
		return apiError(httpStatus, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode set-system-status response: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return classifyCtrlSimple(httpStatus, resp, body)
	}
	return nil
}

// AttachSystemChild adds childID to parentID's children list and delegates
// the write to UpdateSystem (whose M-Z merge preserves every other column).
// The membership is non-exclusive: a child may belong to several parents,
// so attaching is purely additive — an already-present edge is an
// idempotent no-op. The child must exist.
func (c *Client) AttachSystemChild(ctx context.Context, parentID, childID int64) (*System, error) {
	if parentID <= 0 {
		return nil, fmt.Errorf("AttachSystemChild: parentID must be positive, got %d", parentID)
	}
	if childID <= 0 {
		return nil, fmt.Errorf("AttachSystemChild: childID must be positive, got %d", childID)
	}
	if parentID == childID {
		return nil, fmt.Errorf("AttachSystemChild: %w (self-attach: parent=child=%d)", ErrCycleDetected, parentID)
	}
	if _, err := c.GetSystem(ctx, childID); err != nil {
		return nil, fmt.Errorf("AttachSystemChild: fetch child: %w", err)
	}
	parent, err := c.GetSystem(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("AttachSystemChild: fetch parent: %w", err)
	}
	children := splitChildren(deref(parent.Children))
	if containsID(children, childID) {
		return parent, nil
	}
	children = append(children, strconv.FormatInt(childID, 10))
	csv := strings.Join(children, ",")
	return c.UpdateSystem(ctx, &System{ID: &parentID, Children: &csv})
}

// DetachSystemChild removes childID from parentID's children list and
// delegates to UpdateSystem. A missing parent surfaces as ErrNotFound (the
// caller treats it as idempotent success); a child already absent is a
// no-op.
func (c *Client) DetachSystemChild(ctx context.Context, parentID, childID int64) (*System, error) {
	if parentID <= 0 {
		return nil, fmt.Errorf("DetachSystemChild: parentID must be positive, got %d", parentID)
	}
	if childID <= 0 {
		return nil, fmt.Errorf("DetachSystemChild: childID must be positive, got %d", childID)
	}
	parent, err := c.GetSystem(ctx, parentID)
	if err != nil {
		return nil, err
	}
	children := splitChildren(deref(parent.Children))
	if !containsID(children, childID) {
		return parent, nil
	}
	remaining := make([]string, 0, len(children))
	target := strconv.FormatInt(childID, 10)
	for _, id := range children {
		if id != target {
			remaining = append(remaining, id)
		}
	}
	csv := strings.Join(remaining, ",")
	return c.UpdateSystem(ctx, &System{ID: &parentID, Children: &csv})
}

// containsID reports whether the int64 id appears in a slice of id strings.
func containsID(ids []string, id int64) bool {
	target := strconv.FormatInt(id, 10)
	for _, s := range ids {
		if s == target {
			return true
		}
	}
	return false
}

// DeleteSystem soft-deletes an application. ZenTao returns result:success
// even on a re-delete or missing row, so the call is idempotent; a
// not-found reason also collapses to nil.
func (c *Client) DeleteSystem(ctx context.Context, id int64) error {
	body, status, err := c.doControllerForm(ctx, "system", "delete", []string{strconv.FormatInt(id, 10)}, nil, url.Values{})
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
		return fmt.Errorf("decode delete-system response: %w (body=%s)", err, string(body))
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
