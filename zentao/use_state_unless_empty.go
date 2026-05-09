package zentao

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// useStateUnlessEmpty is the plan modifier for Optional+Computed string
// attributes whose server-side semantics are: "if the request body omits or
// blanks the field, ZenTao backfills the requesting account" — e.g.
// product.po. The behaviour we need from the plan, when the user has NOT set a
// value in config, is:
//
//   - prior state has a real value  → pin it (so plan matches Update's
//     refetch and Terraform doesn't see a diff at all)
//   - prior state is empty / null / unknown → set plan Unknown, so the
//     server-assigned default wins on the next apply
//
// The modifier is driven by ConfigValue (unambiguous "did the user set
// this?") rather than PlanValue, because the framework can hand us a non-
// Unknown PlanValue (null or "") in this scenario depending on Terraform Core
// and framework version — gating on PlanValue.IsUnknown() lets that empty
// value through to apply, where the server's "admin" backfill then trips
// "Provider produced inconsistent result after apply: was \"\", but now
// \"admin\"".
func useStateUnlessEmpty() planmodifier.String {
	return useStateUnlessEmptyModifier{}
}

type useStateUnlessEmptyModifier struct{}

func (useStateUnlessEmptyModifier) Description(_ context.Context) string {
	return "When config is null, pin prior state if non-empty, otherwise mark plan Unknown so the server-assigned default wins."
}

func (m useStateUnlessEmptyModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (useStateUnlessEmptyModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// User explicitly set a value in config — leave the plan alone.
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() {
		return
	}

	// Config is null. Pin prior state if it carries a real value; this keeps
	// the plan stable and matches what Update's refetch will return.
	if !req.StateValue.IsNull() && !req.StateValue.IsUnknown() && req.StateValue.ValueString() != "" {
		resp.PlanValue = req.StateValue
		return
	}

	// No usable prior value — leave plan Unknown so the server-assigned
	// default (the requesting account) is what ends up in state on apply.
	resp.PlanValue = types.StringUnknown()
}
