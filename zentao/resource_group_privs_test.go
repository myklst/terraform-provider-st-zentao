package zentao

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestGroupPrivsResource_Metadata(t *testing.T) {
	r := NewGroupPrivsResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_group_privs" {
		t.Fatalf("TypeName = %q, want st-zentao_group_privs", resp.TypeName)
	}
}

func TestGroupPrivsResource_Schema(t *testing.T) {
	r := NewGroupPrivsResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	if a := resp.Schema.Attributes["id"]; !a.IsComputed() || a.IsRequired() || a.IsOptional() {
		t.Errorf("id must be Computed-only")
	}
	for _, req := range []string{"group", "privs"} {
		a := resp.Schema.Attributes[req]
		if !a.IsRequired() || a.IsComputed() || a.IsOptional() {
			t.Errorf("%s must be Required-only (got required=%v computed=%v optional=%v)",
				req, a.IsRequired(), a.IsComputed(), a.IsOptional())
		}
	}
}

// group is RequiresReplace — managePriv keys on a fixed group; re-pointing is
// destroy+create.
func TestGroupPrivsResource_GroupRequiresReplace(t *testing.T) {
	r := NewGroupPrivsResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["group"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("group is not Int64Attribute, got %T", resp.Schema.Attributes["group"])
	}
	want := int64planmodifier.RequiresReplace().Description(context.Background())
	found := false
	for _, pm := range attr.PlanModifiers {
		if pm.Description(context.Background()) == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("group must have RequiresReplace plan modifier")
	}
}

func TestGroupPrivsResource_SetRoundTrip(t *testing.T) {
	ctx := context.Background()
	set, d := setFromStrings(ctx, []string{"story-view", "story-create"})
	if d.HasError() {
		t.Fatalf("setFromStrings: %v", d)
	}
	got, d := setToStrings(ctx, set)
	if d.HasError() {
		t.Fatalf("setToStrings: %v", d)
	}
	if len(got) != 2 {
		t.Fatalf("round trip = %v, want 2 elements", got)
	}
}

// nil slice normalises to an empty (non-null) set so a cleared group reads back
// as `privs = []` rather than null.
func TestGroupPrivsResource_SetFromNilIsEmptyNotNull(t *testing.T) {
	set, d := setFromStrings(context.Background(), nil)
	if d.HasError() {
		t.Fatalf("setFromStrings(nil): %v", d)
	}
	if set.IsNull() {
		t.Fatalf("empty set must not be null")
	}
	if len(set.Elements()) != 0 {
		t.Fatalf("set must be empty, got %d", len(set.Elements()))
	}
}

// null/unknown set flattens to a nil slice (the empty-clear case for SetGroupPrivs).
func TestGroupPrivsResource_SetToStringsNullIsNil(t *testing.T) {
	got, d := setToStrings(context.Background(), types.SetNull(types.StringType))
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got != nil {
		t.Fatalf("null set must flatten to nil, got %v", got)
	}
}

func TestGroupPrivsResource_ModelFromPrivs(t *testing.T) {
	var diags diag.Diagnostics
	m := modelFromPrivs(context.Background(), 42, []string{"story-view"}, &diags)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.ID.ValueString() != "42" {
		t.Errorf("id = %q, want 42", m.ID.ValueString())
	}
	if m.Group.ValueInt64() != 42 {
		t.Errorf("group = %d, want 42", m.Group.ValueInt64())
	}
	if len(m.Privs.Elements()) != 1 {
		t.Errorf("privs len = %d, want 1", len(m.Privs.Elements()))
	}
}

// --- Acceptance tests (TF_ACC=1) ---

func TestAccGroupPrivsResource_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	projectID := groupAccProjectID(t)
	name := shortName("pg-acc-pv")
	config := func(privs string) string {
		return providerBlock() + fmt.Sprintf(`
resource "st-zentao_group" "g" {
  project = %d
  name    = %q
}
resource "st-zentao_group_privs" "p" {
  group = tonumber(st-zentao_group.g.id)
  privs = %s
}`, projectID, name, privs)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: config(`["story-view", "story-create"]`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_group_privs.p", "privs.#", "2"),
					tfresource.TestCheckResourceAttrSet("st-zentao_group_privs.p", "id"),
					tfresource.TestCheckResourceAttrPair("st-zentao_group_privs.p", "id", "st-zentao_group.g", "id"),
				),
			},
			{
				Config: config(`["story-view"]`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_group_privs.p", "privs.#", "1"),
				),
			},
		},
	})
}

func TestAccGroupPrivsResource_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	projectID := groupAccProjectID(t)
	name := shortName("pg-imp-pv")
	config := providerBlock() + fmt.Sprintf(`
resource "st-zentao_group" "g" {
  project = %d
  name    = %q
}
resource "st-zentao_group_privs" "p" {
  group = tonumber(st-zentao_group.g.id)
  privs = ["story-view"]
}`, projectID, name)
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{Config: config},
			{
				ResourceName:      "st-zentao_group_privs.p",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
