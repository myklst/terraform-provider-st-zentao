package zentao

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
// nil when the user did not set them, which the M-Z merge in Update<Entity>
// reads as "preserve baseline".
func optString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func optInt64(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// func optBool(v types.Bool) *bool {
// 	if v.IsNull() || v.IsUnknown() {
// 		return nil
// 	}
// 	b := v.ValueBool()
// 	return &b
// }

// optStrList converts a Terraform list to a *[]string; nil/unknown lists
// become nil ("preserve baseline"), empty lists stay as &[]string{} so the
// M-Z merge can distinguish "user explicitly cleared" from "user did not
// set".
func optStrList(ctx context.Context, v types.List) (*[]string, diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := v.ElementsAs(ctx, &out, true)
	if out == nil {
		out = []string{}
	}
	return &out, diags
}

// deref returns *p when non-nil, otherwise T's zero value. Used to flatten
// pointer fields off the API wrapper into plain Terraform types.
func deref[T any](p *T) T {
	if p == nil {
		var def T
		return def
	}
	return *p
}

// stringListFromSlice normalises a nil slice into an empty list value
// (Terraform distinguishes null-list from empty-list and the latter is
// what `flexibleStringList` decodes to when the wire returns an empty
// string).
func stringListFromSlice(ctx context.Context, src []string) (types.List, diag.Diagnostics) {
	if src == nil {
		src = []string{}
	}
	return types.ListValueFrom(ctx, types.StringType, src)
}
