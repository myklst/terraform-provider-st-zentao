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
	if !resp.Schema.Attributes["id"].IsRequired() {
		t.Error("id must be Required")
	}
	for _, attr := range []string{
		"name", "model", "begin", "end", "program", "products",
		"workflow_group", "multiple", "acl", "pm", "desc",
		"status",
	} {
		a := resp.Schema.Attributes[attr]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", attr)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must not be Required/Optional (Computed-only)", attr)
		}
	}
	// Audit / derived columns must NO longer be in the data source schema.
	for _, gone := range []string{"code", "lifetime", "opened_by", "opened_date", "last_edited_by", "real_began", "real_end", "progress", "team_count", "budget", "budget_unit"} {
		if _, ok := resp.Schema.Attributes[gone]; ok {
			t.Errorf("data source schema must NOT expose %q anymore", gone)
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
