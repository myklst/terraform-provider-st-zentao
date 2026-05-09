package zentao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

// projectAccDefaults are the live-server inputs needed to make a project
// resource pass server validation. ZenTao Max 8.x requires `products`
// (>=1 existing product id) and `workflow_group` (any int the install
// recognises). These default to product id 1 / workflow group 1 to
// match a fresh ZenTao installation; override per host via env vars
// (ZENTAO_TEST_PRODUCT_ID / ZENTAO_TEST_WORKFLOW_GROUP) if the test
// instance differs.
func projectAccDefaults() (productID, workflowGroup int) {
	productID = 1
	workflowGroup = 1
	if v := os.Getenv("ZENTAO_TEST_PRODUCT_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			productID = n
		}
	}
	if v := os.Getenv("ZENTAO_TEST_WORKFLOW_GROUP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			workflowGroup = n
		}
	}
	return
}

func TestProjectResource_Metadata(t *testing.T) {
	r := NewProjectResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_project" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestProjectResource_Schema(t *testing.T) {
	r := NewProjectResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	expected := []string{
		"id", "name", "model", "begin", "end", "program", "products",
		"workflow_group", "multiple", "acl", "pm", "po", "qd", "rd", "desc",
		"code", "status", "lifetime", "opened_by", "opened_date",
		"last_edited_by", "real_began", "real_end", "progress",
		"team_count", "budget", "budget_unit",
	}
	for _, attr := range expected {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing attribute %q", attr)
		}
	}

	// Per upstream module/project/config/form.php + model.php#L2026, products
	// is NOT required on create; only name/model/begin/end/workflow_group are
	// strictly required.
	for _, required := range []string{"name", "model", "begin", "end", "workflow_group"} {
		a := resp.Schema.Attributes[required]
		if !a.IsRequired() {
			t.Errorf("%s must be Required", required)
		}
	}

	for _, computedOnly := range []string{
		"id", "code", "status", "lifetime", "opened_by", "opened_date",
		"last_edited_by", "real_began", "real_end", "progress",
		"team_count", "budget", "budget_unit",
	} {
		a := resp.Schema.Attributes[computedOnly]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computedOnly)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only (got required=%v optional=%v)",
				computedOnly, a.IsRequired(), a.IsOptional())
		}
	}

	for _, optional := range []string{"program", "products", "multiple", "acl", "pm", "po", "qd", "rd", "desc"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)",
				optional, a.IsOptional(), a.IsComputed())
		}
	}
}

// TestProjectResource_ModelRequiresReplace verifies the `model` attribute
// is wired with a RequiresReplace plan modifier. ZenTao does not support
// changing the project model in-place; updating it must trigger a
// replacement of the resource.
func TestProjectResource_ModelRequiresReplace(t *testing.T) {
	r := NewProjectResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["model"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("model attribute is not a StringAttribute, got %T", resp.Schema.Attributes["model"])
	}

	want := stringplanmodifier.RequiresReplace().Description(context.Background())
	found := false
	for _, pm := range attr.PlanModifiers {
		if pm.Description(context.Background()) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("model attribute must have RequiresReplace plan modifier; got %d modifiers", len(attr.PlanModifiers))
	}
}

func TestProjectResource_RoundTrip(t *testing.T) {
	ctx := context.Background()

	productsList, diags := types.ListValueFrom(ctx, types.Int64Type, []int64{42, 7})
	if diags.HasError() {
		t.Fatalf("build products list: %v", diags)
	}

	original := projectResourceModel{
		Name:          types.StringValue("acme"),
		Model:         types.StringValue("scrum"),
		Begin:         types.StringValue("2026-01-01"),
		End:           types.StringValue("2026-12-31"),
		Program:       types.Int64Value(3),
		Products:      productsList,
		WorkflowGroup: types.Int64Value(2),
		ACL:           types.StringValue("private"),
		PM:            types.StringValue("alice"),
		PO:            types.StringValue("bob"),
		QD:            types.StringValue("carol"),
		RD:            types.StringValue("dave"),
		Desc:          types.StringValue("hello"),
	}

	api, diags := original.toAPI(ctx)
	if diags.HasError() {
		t.Fatalf("toAPI diags: %v", diags)
	}
	if api.Name != "acme" || api.Model != "scrum" {
		t.Errorf("name/model wrong: got %q/%q", api.Name, api.Model)
	}
	if api.Parent != 3 {
		t.Errorf("parent wrong: got %d, want 3", api.Parent)
	}
	if api.WorkflowGroup != 2 {
		t.Errorf("workflowGroup wrong: got %d, want 2", api.WorkflowGroup)
	}
	if len(api.Products) != 2 || api.Products[0] != 42 || api.Products[1] != 7 {
		t.Errorf("products wrong: got %v", api.Products)
	}
	if api.Desc != "hello" {
		t.Errorf("desc wrong: got %q", api.Desc)
	}

	// Simulate the wire shape coming back from GetProject (id assigned,
	// server-managed columns populated). Products are intentionally
	// preserved by the Create/Update spliced approach so we mirror that
	// here.
	api.ID = 99
	api.Code = "ACME"
	api.Status = "doing"
	api.OpenedBy = "alice"

	roundtripped, diags := projectFromAPI(ctx, api)
	if diags.HasError() {
		t.Fatalf("projectFromAPI diags: %v", diags)
	}
	if roundtripped.ID.ValueString() != "99" {
		t.Errorf("id wrong: got %q", roundtripped.ID.ValueString())
	}
	if roundtripped.Name.ValueString() != "acme" {
		t.Errorf("name wrong: got %q", roundtripped.Name.ValueString())
	}
	if roundtripped.Program.ValueInt64() != 3 {
		t.Errorf("program (parent) wrong: got %d", roundtripped.Program.ValueInt64())
	}
	if roundtripped.WorkflowGroup.ValueInt64() != 2 {
		t.Errorf("workflow_group wrong: got %d", roundtripped.WorkflowGroup.ValueInt64())
	}
	var gotProducts []int64
	if d := roundtripped.Products.ElementsAs(ctx, &gotProducts, false); d.HasError() {
		t.Fatalf("products extraction: %v", d)
	}
	if len(gotProducts) != 2 || gotProducts[0] != 42 || gotProducts[1] != 7 {
		t.Errorf("products roundtrip wrong: got %v", gotProducts)
	}
	if roundtripped.Status.ValueString() != "doing" {
		t.Errorf("status wrong: got %q", roundtripped.Status.ValueString())
	}
}

func TestProjectFromAPI_NilProductsBecomesEmptyList(t *testing.T) {
	ctx := context.Background()
	p := &zentaoapi.Project{ID: 1, Name: "x", Products: nil}
	m, diags := projectFromAPI(ctx, p)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.Products.IsNull() {
		t.Errorf("products should be empty list, not null")
	}
	var got []int64
	if d := m.Products.ElementsAs(ctx, &got, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}
}

// Acceptance tests below exercise the live ZenTao server when TF_ACC=1.

func TestAccProjectResource_basic(t *testing.T) {
	name := uniqueName("acc-proj")
	productID, workflowGroup := projectAccDefaults()
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_project" "p" {
  name           = %q
  model          = "scrum"
  begin          = "2026-01-01"
  end            = "2026-12-31"
  products       = [%d]
  workflow_group = %d
  desc           = "initial"
}`, name, productID, workflowGroup),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "name", name),
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "model", "scrum"),
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "begin", "2026-01-01"),
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "end", "2026-12-31"),
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "desc", "initial"),
					tfresource.TestCheckResourceAttrSet("st-zentao_project.p", "id"),
					tfresource.TestCheckResourceAttrSet("st-zentao_project.p", "status"),
					tfresource.TestCheckResourceAttrSet("st-zentao_project.p", "opened_by"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_project" "p" {
  name           = %q
  model          = "scrum"
  begin          = "2026-02-01"
  end            = "2026-11-30"
  products       = [%d]
  workflow_group = %d
  desc           = "updated"
}`, name+"-renamed", productID, workflowGroup),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "name", name+"-renamed"),
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "begin", "2026-02-01"),
					tfresource.TestCheckResourceAttr("st-zentao_project.p", "desc", "updated"),
				),
			},
		},
	})
}

func TestAccProjectResource_import(t *testing.T) {
	name := uniqueName("imp-proj")
	productID, workflowGroup := projectAccDefaults()
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_project" "p" {
  name           = %q
  model          = "scrum"
  begin          = "2026-01-01"
  end            = "2026-12-31"
  products       = [%d]
  workflow_group = %d
}`, name, productID, workflowGroup),
			},
			{
				ResourceName:      "st-zentao_project.p",
				ImportState:       true,
				ImportStateVerify: true,
				// products / workflow_group are not echoed back by GetProject,
				// so import cannot rebuild them from server state — verifying
				// every other attribute is enough.
				ImportStateVerifyIgnore: []string{"products", "workflow_group"},
			},
		},
	})
}

// TestAccProjectResource_disappears verifies that an out-of-band delete
// of the project (via the API client, mirroring a manual ZenTao admin
// action) is detected by Read on the next plan, producing a non-empty
// diff so Terraform refresh + plan flags drift.
func TestAccProjectResource_disappears(t *testing.T) {
	name := uniqueName("dis-proj")
	productID, workflowGroup := projectAccDefaults()
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_project" "p" {
  name           = %q
  model          = "scrum"
  begin          = "2026-01-01"
  end            = "2026-12-31"
  products       = [%d]
  workflow_group = %d
}`, name, productID, workflowGroup),
				Check: tfresource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["st-zentao_project.p"]
						if !ok {
							return errors.New("resource missing from state")
						}
						id, err := strconv.Atoi(rs.Primary.ID)
						if err != nil {
							return err
						}
						c, err := zentaoapi.NewClient(os.Getenv("ZENTAO_URL"), os.Getenv("ZENTAO_ACCOUNT"), os.Getenv("ZENTAO_PASSWORD"))
						if err != nil {
							return err
						}
						return c.DeleteProject(context.Background(), id)
					},
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccProjectResource_invalidDateRejected(t *testing.T) {
	name := uniqueName("baddate-proj")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_project" "p" {
  name           = %q
  model          = "scrum"
  begin          = "not-a-date"
  end            = "2026-12-31"
  products       = [1]
  workflow_group = 1
}`, name),
				ExpectError: regexp.MustCompile(`(?i)YYYY-MM-DD`),
			},
		},
	})
}

func TestAccProjectResource_invalidModelRejected(t *testing.T) {
	name := uniqueName("badmodel-proj")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_project" "p" {
  name           = %q
  model          = "not-a-real-model"
  begin          = "2026-01-01"
  end            = "2026-12-31"
  products       = [1]
  workflow_group = 1
}`, name),
				ExpectError: regexp.MustCompile(`(?i)must be one of`),
			},
		},
	})
}
