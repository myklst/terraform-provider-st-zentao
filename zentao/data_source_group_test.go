package zentao

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestGroupDataSource_Metadata(t *testing.T) {
	d := NewGroupDataSource()
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_group" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestGroupDataSource_Schema(t *testing.T) {
	d := NewGroupDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if !resp.Schema.Attributes["id"].IsRequired() {
		t.Error("id must be Required")
	}
	for _, attr := range []string{"project", "name", "role", "desc"} {
		a := resp.Schema.Attributes[attr]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", attr)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must not be Required/Optional (Computed-only)", attr)
		}
	}
}

// groupDSAccProjectID returns the parent project id used by the
// data source acceptance test. Defaults to 28 (the lek-ws.sige.la
// instance probe target) and accepts ZENTAO_TEST_PROJECT_ID overrides
// for other hosts.
//
// Defined locally inside this test file to avoid colliding with a
// same-named helper that the parallel resource agent may add to
// resource_group_test.go. If both helpers exist at compile
// time, the one in this file is the only one referenced from this
// file's tests, so there's no ambiguity.
func groupDSAccProjectID() int {
	id := 28
	if v := os.Getenv("ZENTAO_TEST_PROJECT_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			id = n
		}
	}
	return id
}

// TestAccGroupDataSource_LookupExisting creates a project group
// via the resource and looks it up through the data source in the same
// step. Mirrors TestAccProgramDataSource_basic. Uses the `tf-acc-pgd-`
// name prefix so the cleanup grep can distinguish data source acc test
// rows from the resource acc test rows (`tf-acc-pg-`).
func TestAccGroupDataSource_LookupExisting(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set; skipping live-server acceptance test")
	}
	projectID := groupDSAccProjectID()
	// Must fit zt_group.name (varchar(30)). UnixNano (19 digits) is too
	// long for the rename pattern; keep it tight here too for symmetry
	// with the resource acc test.
	name := fmt.Sprintf("pgd-acc-%d", time.Now().Unix())
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_group" "src" {
  project = %d
  name    = %q
  role    = "dev"
  desc    = "data source acceptance test"
}

data "st-zentao_group" "lookup" {
  id = st-zentao_group.src.id
}
`, projectID, name),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("data.st-zentao_group.lookup", "name", name),
					tfresource.TestCheckResourceAttr("data.st-zentao_group.lookup", "role", "dev"),
					tfresource.TestCheckResourceAttr("data.st-zentao_group.lookup", "desc", "data source acceptance test"),
					tfresource.TestCheckResourceAttr("data.st-zentao_group.lookup", "project", strconv.Itoa(projectID)),
					tfresource.TestCheckResourceAttrPair("data.st-zentao_group.lookup", "id", "st-zentao_group.src", "id"),
				),
			},
		},
	})
}
