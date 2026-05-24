package zentao

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// systemAccProductID is the owning product the acc tests create the
// application under. Defaults to a fresh-install product; override per host.
func systemAccProductID() int {
	productID := 1
	if v := os.Getenv("ZENTAO_TEST_PRODUCT_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			productID = n
		}
	}
	return productID
}

func TestSystemResource_Metadata(t *testing.T) {
	r := NewSystemResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_system" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestSystemResource_Schema(t *testing.T) {
	r := NewSystemResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	for _, required := range []string{"product", "name"} {
		if !resp.Schema.Attributes[required].IsRequired() {
			t.Errorf("%s must be Required", required)
		}
	}
	// status + desc are user-settable but server-defaulted.
	for _, optional := range []string{"desc", "status"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)", optional, a.IsOptional(), a.IsComputed())
		}
	}
	// Server-managed, read-only.
	for _, computedOnly := range []string{"id", "integrated", "created_by", "created_date", "last_edited_by", "last_edited_date"} {
		a := resp.Schema.Attributes[computedOnly]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computedOnly)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only (got required=%v optional=%v)", computedOnly, a.IsRequired(), a.IsOptional())
		}
	}

	// product is immutable — re-pointing the owning product is a replace.
	prod := resp.Schema.Attributes["product"].(schema.Int64Attribute)
	if len(prod.PlanModifiers) == 0 {
		t.Error("product must carry a RequiresReplace plan modifier")
	}

	// last_edited_* are recomputed by the server on every edit, so they
	// must NOT carry UseStateForUnknown — pinning prior state causes
	// "inconsistent result after apply" on Update.
	for _, recomputed := range []string{"last_edited_by", "last_edited_date"} {
		a := resp.Schema.Attributes[recomputed].(schema.StringAttribute)
		if len(a.PlanModifiers) != 0 {
			t.Errorf("%s must have no plan modifiers (server recomputes it on every edit)", recomputed)
		}
	}
}

func TestAccSystemResource_basic(t *testing.T) {
	name := uniqueName("acc-sys")
	productID := systemAccProductID()
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_system" "s" {
  product = %d
  name    = %q
  desc    = "initial"
}`, productID, name),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "name", name),
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "desc", "initial"),
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "product", strconv.Itoa(productID)),
					tfresource.TestCheckResourceAttrSet("st-zentao_system.s", "id"),
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "status", "active"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_system" "s" {
  product = %d
  name    = %q
  desc    = "updated"
  status  = "inactive"
}`, productID, name+"-renamed"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "name", name+"-renamed"),
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "desc", "updated"),
					tfresource.TestCheckResourceAttr("st-zentao_system.s", "status", "inactive"),
				),
			},
		},
	})
}

func TestAccSystemResource_import(t *testing.T) {
	name := uniqueName("imp-sys")
	productID := systemAccProductID()
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_system" "s" {
  product = %d
  name    = %q
}`, productID, name),
			},
			{
				ResourceName:      "st-zentao_system.s",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
