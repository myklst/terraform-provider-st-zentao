package zentao

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestProjectDataSource_Schema(t *testing.T) {
	d := NewProjectDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
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
	if !resp.Schema.Attributes["id"].IsRequired() {
		t.Error("id must be Required")
	}
	for _, attr := range expected {
		if attr == "id" {
			continue
		}
		a := resp.Schema.Attributes[attr]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", attr)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must not be Required/Optional (Computed-only)", attr)
		}
	}
}

func TestProjectDataSource_Metadata(t *testing.T) {
	d := NewProjectDataSource()
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_project" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}
