package zentao

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestSystemChildAttachmentResource_Metadata(t *testing.T) {
	r := NewSystemChildAttachmentResource()
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_system_child_attachment" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestSystemChildAttachmentResource_Schema(t *testing.T) {
	r := NewSystemChildAttachmentResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	// Both edge fields are Required and RequiresReplace — the edge is
	// defined by the (parent, child) pair, with no mutable attribute.
	for _, name := range []string{"parent", "child"} {
		a := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !a.IsRequired() {
			t.Errorf("%s must be Required", name)
		}
		if len(a.PlanModifiers) == 0 {
			t.Errorf("%s must carry a RequiresReplace plan modifier", name)
		}
	}
	if !resp.Schema.Attributes["id"].IsComputed() {
		t.Error("id must be Computed")
	}
}

func TestSystemEdgeID(t *testing.T) {
	if got := systemEdgeID(691, 693); got != "691-693" {
		t.Errorf("systemEdgeID = %q, want 691-693", got)
	}
}

func TestChildInList(t *testing.T) {
	cases := []struct {
		csv  string
		id   int64
		want bool
	}{
		{"686,693", 693, true},
		{"686, 693", 693, true},
		{"686,693", 687, false},
		{"", 693, false},
		{"6930", 693, false},
	}
	for _, tc := range cases {
		if got := childInList(tc.csv, tc.id); got != tc.want {
			t.Errorf("childInList(%q,%d) = %v, want %v", tc.csv, tc.id, got, tc.want)
		}
	}
}

func TestSystemAttachPair(t *testing.T) {
	cases := []struct {
		parent, child string
		wantOK        bool
	}{
		{"691", "693", true},
		{"0", "693", false},
		{"691", "0", false},
		{"x", "693", false},
		{"691", "y", false},
	}
	for _, tc := range cases {
		var d diag.Diagnostics
		_, _, ok := systemAttachPair(tc.parent, tc.child, &d)
		if ok != tc.wantOK {
			t.Errorf("systemAttachPair(%q,%q) ok = %v, want %v", tc.parent, tc.child, ok, tc.wantOK)
		}
	}
}

func TestAccSystemChildAttachmentResource_basic(t *testing.T) {
	parentName := uniqueName("acc-sys-par")
	childName := uniqueName("acc-sys-chi")
	productID := systemAccProductID()
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []tfresource.TestStep{
			{
				Config: providerBlock() + fmt.Sprintf(`
resource "st-zentao_system" "parent" {
  product = %d
  name    = %q
}
resource "st-zentao_system" "child" {
  product = %d
  name    = %q
}
resource "st-zentao_system_child_attachment" "edge" {
  parent = st-zentao_system.parent.id
  child  = st-zentao_system.child.id
}`, productID, parentName, productID, childName),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttrSet("st-zentao_system_child_attachment.edge", "id"),
					tfresource.TestCheckResourceAttrPair("st-zentao_system_child_attachment.edge", "parent", "st-zentao_system.parent", "id"),
					tfresource.TestCheckResourceAttrPair("st-zentao_system_child_attachment.edge", "child", "st-zentao_system.child", "id"),
				),
			},
			{
				ResourceName:      "st-zentao_system_child_attachment.edge",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSystemChildAttachmentResource_selfAttachRejected(t *testing.T) {
	name := uniqueName("acc-sys-self")
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
}
resource "st-zentao_system_child_attachment" "edge" {
  parent = st-zentao_system.s.id
  child  = st-zentao_system.s.id
}`, productID, name),
				ExpectError: regexp.MustCompile(`(?i)self-attach`),
			},
		},
	})
}
