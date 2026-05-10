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
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

func TestProgramParentAttachmentResource_Schema(t *testing.T) {
	r := NewProgramParentAttachmentResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, required := range []string{"program", "parent"} {
		if !resp.Schema.Attributes[required].IsRequired() {
			t.Errorf("%s must be Required", required)
		}
	}
	if !resp.Schema.Attributes["id"].IsComputed() {
		t.Error("id must be Computed")
	}
}

func TestProgramParentAttachmentResource_Metadata(t *testing.T) {
	r := NewProgramParentAttachmentResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_program_parent_attachment" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

// ---- acceptance ----

// Two programs, one attaches to the other; verify, then re-parent to a
// third top-level program, then tear down. The for_each pattern proves
// the original use case (self-reference cycle workaround).
func TestAccProgramParentAttachmentResource_basic(t *testing.T) {
	parentName := uniqueName("acc-pp-parent")
	childName := uniqueName("acc-pp-child")
	otherName := uniqueName("acc-pp-other")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "parent" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program" "child" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program_parent_attachment" "att" {
  program = st-zentao_program.child.id
  parent  = st-zentao_program.parent.id
}`, parentName, childName),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttrSet("st-zentao_program_parent_attachment.att", "id"),
					tfresource.TestCheckResourceAttrPair(
						"st-zentao_program_parent_attachment.att", "program",
						"st-zentao_program.child", "id",
					),
					tfresource.TestCheckResourceAttrPair(
						"st-zentao_program_parent_attachment.att", "parent",
						"st-zentao_program.parent", "id",
					),
				),
			},
			// Re-parent: in-place Update to a different parent.
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "parent" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program" "child" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program" "other" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program_parent_attachment" "att" {
  program = st-zentao_program.child.id
  parent  = st-zentao_program.other.id
}`, parentName, childName, otherName),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttrPair(
						"st-zentao_program_parent_attachment.att", "parent",
						"st-zentao_program.other", "id",
					),
				),
			},
		},
	})
}

func TestAccProgramParentAttachmentResource_selfAttachRejected(t *testing.T) {
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + `
resource "st-zentao_program_parent_attachment" "att" {
  program = "5"
  parent  = "5"
}`,
				ExpectError: regexp.MustCompile(`(?i)self-attach`),
			},
		},
	})
}

func TestAccProgramParentAttachmentResource_zeroParentRejected(t *testing.T) {
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + `
resource "st-zentao_program_parent_attachment" "att" {
  program = "5"
  parent  = "0"
}`,
				ExpectError: regexp.MustCompile(`(?i)not a valid attachment`),
			},
		},
	})
}

func TestAccProgramParentAttachmentResource_nonNumericRejected(t *testing.T) {
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + `
resource "st-zentao_program_parent_attachment" "att" {
  program = "abc"
  parent  = "5"
}`,
				ExpectError: regexp.MustCompile(`(?i)positive integer string`),
			},
		},
	})
}

func TestAccProgramParentAttachmentResource_import(t *testing.T) {
	parentName := uniqueName("acc-pp-imp-parent")
	childName := uniqueName("acc-pp-imp-child")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "parent" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program" "child" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program_parent_attachment" "att" {
  program = st-zentao_program.child.id
  parent  = st-zentao_program.parent.id
}`, parentName, childName),
			},
			{
				ResourceName:      "st-zentao_program_parent_attachment.att",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Out-of-band detach: remove the parent in ZenTao directly, then re-plan.
// Read should drop the attachment and plan should recreate it.
func TestAccProgramParentAttachmentResource_outOfBandDetach(t *testing.T) {
	parentName := uniqueName("acc-pp-oob-parent")
	childName := uniqueName("acc-pp-oob-child")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "parent" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program" "child" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}
resource "st-zentao_program_parent_attachment" "att" {
  program = st-zentao_program.child.id
  parent  = st-zentao_program.parent.id
}`, parentName, childName),
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources["st-zentao_program_parent_attachment.att"]
					if !ok {
						return errors.New("attachment missing from state")
					}
					child, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
					if err != nil {
						return err
					}
					c, err := zentaoapi.NewClient(os.Getenv("ZENTAO_URL"), os.Getenv("ZENTAO_ACCOUNT"), os.Getenv("ZENTAO_PASSWORD"))
					if err != nil {
						return err
					}
					return c.SetProgramParent(context.Background(), child, 0)
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
