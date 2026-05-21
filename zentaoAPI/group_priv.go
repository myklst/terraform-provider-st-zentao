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
// project-managePriv-{projectID}-{groupID}.json. Only selectedPrivList — the
// group's granted set, already flat "module-method" strings — is consumed; the
// ~470-entry allPrivList catalog and the redundant nested groupPrivs map are
// ignored (encoding/json drops unknown keys).
type managePrivData struct {
	SelectedPrivList []string `json:"selectedPrivList"`
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

// GetGroupPrivs returns a group's privilege grants as flat "module-method"
// strings. Both system and project-scoped groups read through the project
// module's managePriv action (projectID 0 for system groups).
func (c *Client) GetGroupPrivs(ctx context.Context, groupID int64) ([]string, error) {
	args, err := c.managePrivPathArgs(ctx, groupID)
	if err != nil {
		return nil, err
	}
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
	return inner.SelectedPrivList, nil
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

// SetGroupPrivs replaces a group's entire privilege set. Priv format is
// validated before any network round trip; an empty set clears all grants.
func (c *Client) SetGroupPrivs(ctx context.Context, groupID int64, privs []string) error {
	form, err := privsToForm(privs)
	if err != nil {
		return fmt.Errorf("SetGroupPrivs: %w", err)
	}
	args, err := c.managePrivPathArgs(ctx, groupID)
	if err != nil {
		return err
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
