package zentao

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// useStateUnlessEmpty mirrors UseStateForUnknown for Optional+Computed string
// attributes, but skips the pin when the prior state value is the empty
// string. Use it for fields where ZenTao backfills a server-side default
// (typically the requesting account) when the request body omits the field —
// e.g. product.po/qd/rd. Pinning "" from state would force the next plan to
// say `po=""` while Update's refetch returns `"admin"`, raising
// `Provider produced inconsistent result after apply`.
func useStateUnlessEmpty() planmodifier.String {
	return useStateUnlessEmptyModifier{}
}

type useStateUnlessEmptyModifier struct{}

func (useStateUnlessEmptyModifier) Description(_ context.Context) string {
	return "Use prior state for unknown plan values, except when the state value is the empty string."
}

func (m useStateUnlessEmptyModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (useStateUnlessEmptyModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.StateValue.ValueString() == "" {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}
