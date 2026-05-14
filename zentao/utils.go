package zentao

import "github.com/hashicorp/terraform-plugin-framework/types"

func commaJoin(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += `"` + s + `"`
	}
	return out
}

// optString returns nil for null/unknown plan values, &v otherwise. Required
// schema fields produce non-nil pointers; Optional+Computed fields produce
// nil when the user did not set them, which the M-Z merge in UpdateProgram
// reads as "preserve baseline".
func optString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
