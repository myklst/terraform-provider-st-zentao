package zentao

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestWorkflowGroupDataSource_Metadata(t *testing.T) {
	d := NewWorkflowGroupDataSource()
	var resp datasource.MetadataResponse
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "st-zentao"}, &resp)
	if resp.TypeName != "st-zentao_workflow_group" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

func TestWorkflowGroupDataSource_Schema(t *testing.T) {
	d := NewWorkflowGroupDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	for _, required := range []string{"project_model", "project_type"} {
		a := resp.Schema.Attributes[required]
		if !a.IsRequired() {
			t.Errorf("%s must be Required", required)
		}
	}
	for _, computed := range []string{"id", "code", "name"} {
		a := resp.Schema.Attributes[computed]
		if !a.IsComputed() {
			t.Errorf("%s must be Computed", computed)
		}
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%s must be Computed-only", computed)
		}
	}
}
