package zentao

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// shortName returns a name short enough to fit zt_group.name (varchar(30))
// even when the test suffixes "-r" for the rename step. Format:
// "<prefix>-<unix>" → e.g. "pg-acc-1778288393" (17 chars + "-r" → 19 chars).
// uniqueName(prefix) elsewhere uses UnixNano (19 digits) which busts the
// 30-char column on rename-step suffixing.
func shortName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().Unix())
}

// groupAccProjectID returns the parent project id used by acceptance
// tests. Defaults to 28 (the lek-ws.sige.la test instance has a known
// project at this id; mirrors the spec's recommendation in §4.9). NEVER
// returns 0 — project=0 is reserved for system groups and the resource
// itself rejects it.
func groupAccProjectID(t *testing.T) int {
	t.Helper()
	id := 28
	if v := os.Getenv("ZENTAO_TEST_PROJECT_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			id = n
		}
	}
	if id <= 0 {
		t.Fatalf("project id must be > 0; got %d", id)
	}
	return id
}

func TestGroupResource_Metadata(t *testing.T) {
	r := NewGroupResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_group" {
		t.Fatalf("TypeName = %q, want st-zentao_group", resp.TypeName)
	}
}

func TestGroupResource_Schema(t *testing.T) {
	r := NewGroupResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	// Required-only attributes. After the system/project unification
	// `project` is Optional+Computed (defaults to 0 → system group), so
	// only `name` is Required-only now.
	{
		a := resp.Schema.Attributes["name"]
		if !a.IsRequired() {
			t.Errorf("name must be Required")
		}
		if a.IsComputed() || a.IsOptional() {
			t.Errorf("name must be Required-only (got computed=%v optional=%v)",
				a.IsComputed(), a.IsOptional())
		}
	}

	// Computed-only.
	{
		a := resp.Schema.Attributes["id"]
		if !a.IsComputed() {
			t.Errorf("id must be Computed")
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("id must be Computed-only (got required=%v optional=%v)",
				a.IsRequired(), a.IsOptional())
		}
	}

	// Optional+Computed: role, desc, and project (default 0 = system flavour).
	for _, optional := range []string{"project", "role", "desc"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)",
				optional, a.IsOptional(), a.IsComputed())
		}
	}
}

// TestGroupResource_ProjectRequiresReplace verifies the `project`
// attribute is wired with a RequiresReplace plan modifier. ZenTao does not
// document a way to move a permission group between projects, so changing
// it must trigger replacement.
func TestGroupResource_ProjectRequiresReplace(t *testing.T) {
	r := NewGroupResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["project"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("project attribute is not an Int64Attribute, got %T", resp.Schema.Attributes["project"])
	}

	want := int64planmodifier.RequiresReplace().Description(context.Background())
	found := false
	for _, pm := range attr.PlanModifiers {
		if pm.Description(context.Background()) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("project attribute must have RequiresReplace plan modifier; got %d modifiers", len(attr.PlanModifiers))
	}
}

func TestGroupResource_RoundTrip(t *testing.T) {
	original := groupResourceModel{
		Project: types.Int64Value(28),
		Name:    types.StringValue("acme-team"),
		Role:    types.StringValue("dev"),
		Desc:    types.StringValue("acme description"),
	}

	api := original.toAPI()
	if api.Project != 28 {
		t.Errorf("project wrong: got %d, want 28", api.Project)
	}
	if api.Name != "acme-team" {
		t.Errorf("name wrong: got %q", api.Name)
	}
	if api.Role != "dev" {
		t.Errorf("role wrong: got %q", api.Role)
	}
	if api.Desc != "acme description" {
		t.Errorf("desc wrong: got %q", api.Desc)
	}

	// Simulate the wire shape coming back from GetGroup with the
	// id assigned and other fields untouched.
	api.ID = 10000007

	roundtripped := groupFromAPI(api)
	if roundtripped.ID.ValueString() != "10000007" {
		t.Errorf("id wrong: got %q", roundtripped.ID.ValueString())
	}
	if roundtripped.Project.ValueInt64() != 28 {
		t.Errorf("project wrong: got %d", roundtripped.Project.ValueInt64())
	}
	if roundtripped.Name.ValueString() != "acme-team" {
		t.Errorf("name wrong: got %q", roundtripped.Name.ValueString())
	}
	if roundtripped.Role.ValueString() != "dev" {
		t.Errorf("role wrong: got %q", roundtripped.Role.ValueString())
	}
	if roundtripped.Desc.ValueString() != "acme description" {
		t.Errorf("desc wrong: got %q", roundtripped.Desc.ValueString())
	}
}

// Acceptance tests below exercise the live ZenTao server when TF_ACC=1.

func TestAccGroupResource_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	projectID := groupAccProjectID(t)
	// shortName + "-r" must fit zt_group.name (varchar(30)).
	name := shortName("pg-acc")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_group" "g" {
  project = %d
  name    = %q
  role    = "dev"
  desc    = "acceptance test"
}`, projectID, name),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_group.g", "project", strconv.Itoa(projectID)),
					tfresource.TestCheckResourceAttr("st-zentao_group.g", "name", name),
					tfresource.TestCheckResourceAttr("st-zentao_group.g", "role", "dev"),
					tfresource.TestCheckResourceAttr("st-zentao_group.g", "desc", "acceptance test"),
					tfresource.TestCheckResourceAttrSet("st-zentao_group.g", "id"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_group" "g" {
  project = %d
  name    = %q
  role    = "dev"
  desc    = "updated description"
}`, projectID, name+"-r"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_group.g", "name", name+"-r"),
					tfresource.TestCheckResourceAttr("st-zentao_group.g", "desc", "updated description"),
				),
			},
		},
	})
}

func TestAccGroupResource_import(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping acceptance test")
	}
	projectID := groupAccProjectID(t)
	name := shortName("pg-imp")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_group" "g" {
  project = %d
  name    = %q
  role    = "dev"
  desc    = "import roundtrip"
}`, projectID, name),
			},
			{
				ResourceName:      "st-zentao_group.g",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
