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

func TestProgramResource_Schema(t *testing.T) {
	r := NewProgramResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	expected := []string{
		"id", "code", "name", "begin", "end", "pm", "description",
		"status", "parent", "type", "category", "acl", "po", "qd", "rd",
		"budget", "budget_unit", "opened_by", "opened_date",
		"real_began", "real_end", "progress", "team_count",
	}
	for _, attr := range expected {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing attribute %q", attr)
		}
	}
	for _, required := range []string{"name", "begin", "end"} {
		if !resp.Schema.Attributes[required].IsRequired() {
			t.Errorf("%s must be Required", required)
		}
	}
	for _, computedOnly := range []string{
		"id", "code", "status", "parent", "type", "category",
		"acl", "po", "qd", "rd", "budget", "budget_unit",
		"opened_by", "opened_date", "real_began", "real_end",
		"progress", "team_count",
	} {
		a := resp.Schema.Attributes[computedOnly]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computedOnly)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only (got required=%v optional=%v)", computedOnly, a.IsRequired(), a.IsOptional())
		}
	}
	for _, optional := range []string{"pm", "description"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed (got optional=%v computed=%v)", optional, a.IsOptional(), a.IsComputed())
		}
	}
}

func TestProgramResource_Metadata(t *testing.T) {
	r := NewProgramResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_program" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestAccProgramResource_basic(t *testing.T) {
	name := uniqueName("acc-prog")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name        = %q
  begin       = "2026-01-01"
  end         = "2026-12-31"
  description = "initial"
}`, name),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "name", name),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "begin", "2026-01-01"),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "end", "2026-12-31"),
					tfresource.TestCheckResourceAttrSet("st-zentao_program.p", "id"),
					tfresource.TestCheckResourceAttrSet("st-zentao_program.p", "status"),
					tfresource.TestCheckResourceAttrSet("st-zentao_program.p", "opened_by"),
				),
			},
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name        = %q
  begin       = "2026-02-01"
  end         = "2026-11-30"
  description = "updated"
}`, name+"-renamed"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "name", name+"-renamed"),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "begin", "2026-02-01"),
					tfresource.TestCheckResourceAttr("st-zentao_program.p", "description", "updated"),
				),
			},
		},
	})
}

func TestAccProgramResource_invalidDateRejected(t *testing.T) {
	name := uniqueName("baddate")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "01/01/2026"
  end   = "2026-12-31"
}`, name),
				ExpectError: regexp.MustCompile(`(?i)YYYY-MM-DD`),
			},
		},
	})
}

func TestAccProgramResource_import(t *testing.T) {
	name := uniqueName("imp-prog")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}`, name),
			},
			{
				ResourceName:      "st-zentao_program.p",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccProgramResource_disappears(t *testing.T) {
	name := uniqueName("dis-prog")
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_program" "p" {
  name  = %q
  begin = "2026-01-01"
  end   = "2026-12-31"
}`, name),
				Check: tfresource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["st-zentao_program.p"]
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
						return c.DeleteProgram(context.Background(), id)
					},
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
