package zentao

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProvider_Schema_HasExpectedAttributes(t *testing.T) {
	p := New()
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"url", "account", "password"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if !resp.Schema.Attributes["password"].IsSensitive() {
		t.Error("password must be sensitive")
	}
}

func TestProvider_Metadata(t *testing.T) {
	p := New()
	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)
	if resp.TypeName != "st-zentao" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestProductResource_Schema(t *testing.T) {
	r := NewProductResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	expected := []string{
		"id", "code", "name", "program", "line", "type", "desc",
		"acl", "po", "qd", "rd", "reviewer",
		"status", "created_by", "created_date", "program_name",
	}
	for _, attr := range expected {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing attribute %q", attr)
		}
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name must be Required")
	}
	for _, computedOnly := range []string{"id", "code", "status", "created_by", "created_date", "program_name"} {
		a := resp.Schema.Attributes[computedOnly]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computedOnly)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only (got required=%v optional=%v)", computedOnly, a.IsRequired(), a.IsOptional())
		}
	}
	for _, optional := range []string{"program", "line", "type", "desc", "acl", "po", "qd", "rd", "reviewer"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)", optional, a.IsOptional(), a.IsComputed())
		}
	}
}

// program_name is server-derived from `program`. Pinning it via
// UseStateForUnknown causes "Provider produced inconsistent result after apply"
// when `program` changes (state holds the old program's name; Update returns
// the new one). Regression guard for that bug.
func TestProductResource_ProgramName_NoUseStateForUnknown(t *testing.T) {
	r := NewProductResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	a, ok := resp.Schema.Attributes["program_name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("program_name must be a StringAttribute, got %T", resp.Schema.Attributes["program_name"])
	}
	if len(a.PlanModifiers) != 0 {
		t.Errorf("program_name must have no plan modifiers (got %d) — UseStateForUnknown causes inconsistent-after-apply when `program` changes", len(a.PlanModifiers))
	}
}

// po/qd/rd are role-username Optional+Computed fields where ZenTao backfills
// the requesting account when the request body omits the field. Pinning ""
// from state via UseStateForUnknown would force the next plan to claim
// `po=""`, while Update's refetch returns `"admin"` — Terraform then raises
// "Provider produced inconsistent result after apply". Regression guard:
// each field carries exactly one plan modifier (useStateUnlessEmpty), and
// that modifier must obey config/state semantics across all PlanValue inputs
// the framework may hand us (Unknown, null, or "" — behaviour varies between
// Terraform Core and plugin-framework versions).
func TestProductResource_RoleFields_UseStateUnlessEmpty(t *testing.T) {
	r := NewProductResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	for _, name := range []string{"po"} {
		a, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s must be a StringAttribute, got %T", name, resp.Schema.Attributes[name])
		}
		if len(a.PlanModifiers) != 1 {
			t.Errorf("%s expected exactly 1 plan modifier, got %d", name, len(a.PlanModifiers))
			continue
		}
		mod := a.PlanModifiers[0]

		cases := []struct {
			label    string
			config   types.String
			state    types.String
			plan     types.String
			wantPin  string // "" means assert Unknown
			unknown  bool
			leaveAs  bool   // assert plan stays equal to input plan
			leaveVal string // expected literal value when leaveAs is true
		}{
			// User removed `po` from config; state held a real value. Plan can
			// arrive as Unknown OR as the empty string depending on framework
			// behaviour — both must end up pinned to the state value, otherwise
			// Update's "admin" backfill triggers inconsistent-after-apply.
			{label: "config-null/state-set/plan-unknown", config: types.StringNull(), state: types.StringValue("admin"), plan: types.StringUnknown(), wantPin: "admin"},
			{label: "config-null/state-set/plan-empty", config: types.StringNull(), state: types.StringValue("admin"), plan: types.StringValue(""), wantPin: "admin"},
			{label: "config-null/state-set/plan-null", config: types.StringNull(), state: types.StringValue("admin"), plan: types.StringNull(), wantPin: "admin"},
			// State carries no usable value — leave plan Unknown so the server
			// default wins regardless of what PlanValue arrived as.
			{label: "config-null/state-empty", config: types.StringNull(), state: types.StringValue(""), plan: types.StringUnknown(), unknown: true},
			{label: "config-null/state-null", config: types.StringNull(), state: types.StringNull(), plan: types.StringUnknown(), unknown: true},
			// User set a value in config — modifier must not touch the plan.
			{label: "config-set/state-set", config: types.StringValue("bob"), state: types.StringValue("admin"), plan: types.StringValue("bob"), leaveAs: true, leaveVal: "bob"},
		}

		for _, tc := range cases {
			r := planmodifier.StringResponse{PlanValue: tc.plan}
			mod.PlanModifyString(context.Background(),
				planmodifier.StringRequest{
					ConfigValue: tc.config,
					StateValue:  tc.state,
					PlanValue:   tc.plan,
				}, &r)

			switch {
			case tc.unknown:
				if !r.PlanValue.IsUnknown() {
					t.Errorf("%s/%s: want Unknown, got %v", name, tc.label, r.PlanValue)
				}
			case tc.leaveAs:
				if r.PlanValue.ValueString() != tc.leaveVal {
					t.Errorf("%s/%s: want %q untouched, got %v", name, tc.label, tc.leaveVal, r.PlanValue)
				}
			default:
				if r.PlanValue.IsUnknown() || r.PlanValue.IsNull() || r.PlanValue.ValueString() != tc.wantPin {
					t.Errorf("%s/%s: want pinned %q, got %v", name, tc.label, tc.wantPin, r.PlanValue)
				}
			}
		}
	}
}

func TestProductResource_Metadata(t *testing.T) {
	r := NewProductResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_product" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}
