package zentaoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// managePrivData is the inner payload of
// project-managePriv-{projectID}-{groupID}.json. SelectedPrivList is the
// group's granted set; AllPrivList is the catalog of every priv assignable to
// the group in its scope. Both are flat "module-method" strings. The redundant
// nested groupPrivs map and the rest of the ~13 keys are ignored (encoding/json
// drops unknown keys).
type managePrivData struct {
	SelectedPrivList []string `json:"selectedPrivList"`
	AllPrivList      []string `json:"allPrivList"`
}

// managePrivPathArgs resolves the {projectID}-{groupID} path args for a group's
// managePriv route. The project column (0 for system groups) comes from
// GetGroup, which also supplies the only reliable not-found signal — managePriv
// itself never 404s.
func (c *Client) managePrivPathArgs(ctx context.Context, groupID int64) ([]string, error) {
	g, err := c.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return []string{
		strconv.FormatInt(deref(g.Project), 10),
		strconv.FormatInt(groupID, 10),
	}, nil
}

// fetchManagePriv GETs the managePriv payload (granted set + assignable
// catalog) for a group identified by the given path args.
func (c *Client) fetchManagePriv(ctx context.Context, args []string) (*managePrivData, error) {
	body, status, err := c.doController(ctx, "project", "managePriv", args, nil, nil)
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
		return nil, fmt.Errorf("decode group-privs envelope: %w (body=%s)", err, string(body))
	}
	if env.Status != "success" {
		return nil, classifyCtrlError(status, env, body)
	}
	var inner managePrivData
	if err := env.DecodeData(&inner); err != nil {
		return nil, fmt.Errorf("decode group-privs data: %w (body=%s)", err, string(body))
	}
	return &inner, nil
}

// GetGroupPrivs returns a group's privilege grants as flat "module-method"
// strings. Both system and project-scoped groups read through the project
// module's managePriv action (projectID 0 for system groups).
func (c *Client) GetGroupPrivs(ctx context.Context, groupID int64) ([]string, error) {
	args, err := c.managePrivPathArgs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	d, err := c.fetchManagePriv(ctx, args)
	if err != nil {
		return nil, err
	}
	return d.SelectedPrivList, nil
}

// privsToForm builds the managePriv POST body. managePriv is replace-all, so
// the form carries the complete desired set. `noChecked=1` is ALWAYS present:
// the controller saves only when $_POST is non-empty, so an empty body (the
// empty-set case) would re-render the form instead of clearing the grants.
// Each "module-method" priv splits on the FIRST hyphen into
// actions[<module>][]=<method>.
func privsToForm(privs []string) (url.Values, error) {
	form := url.Values{}
	form.Set("noChecked", "1")
	for _, p := range privs {
		module, method, ok := strings.Cut(p, "-")
		if !ok || module == "" || method == "" {
			return nil, fmt.Errorf("invalid priv %q: want \"module-method\"", p)
		}
		form.Add("actions["+module+"][]", method)
	}
	return form, nil
}

// SetGroupPrivs replaces a group's entire privilege set. It rejects privs the
// server would silently drop: the managePriv POST returns success but persists
// only privs present in the group's assignable catalog (allPrivList), so an
// unknown priv would otherwise re-read as missing and trip Terraform's
// "inconsistent result after apply". Format is validated before any round trip;
// an empty set clears all grants.
func (c *Client) SetGroupPrivs(ctx context.Context, groupID int64, privs []string) error {
	form, err := privsToForm(privs)
	if err != nil {
		return fmt.Errorf("SetGroupPrivs: %w", err)
	}
	args, err := c.managePrivPathArgs(ctx, groupID)
	if err != nil {
		return err
	}
	current, err := c.fetchManagePriv(ctx, args)
	if err != nil {
		return err
	}
	if unknown := privsNotIn(privs, current.AllPrivList); len(unknown) > 0 {
		return fmt.Errorf("SetGroupPrivs: privs not assignable to group %d: %s "+
			"(not in the group's privilege catalog — the server would silently drop them)",
			groupID, strings.Join(unknown, ", "))
	}
	body, status, err := c.doControllerForm(ctx, "project", "managePriv", args, nil, form)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= 400 {
		return apiError(status, body)
	}
	var resp CtrlSimpleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode set-group-privs envelope: %w (body=%s)", err, string(body))
	}
	if !resp.IsSuccess() {
		return classifyCtrlSimple(status, resp, body)
	}
	return nil
}

// privsNotIn returns the want entries absent from catalog, preserving order.
func privsNotIn(want, catalog []string) []string {
	allowed := make(map[string]struct{}, len(catalog))
	for _, p := range catalog {
		allowed[p] = struct{}{}
	}
	var missing []string
	for _, p := range want {
		if _, ok := allowed[p]; !ok {
			missing = append(missing, p)
		}
	}
	return missing
}
