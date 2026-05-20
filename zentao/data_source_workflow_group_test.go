package zentao

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
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

	if a := resp.Schema.Attributes["type"]; !a.IsRequired() {
		t.Errorf("type must be Required")
	}
	for _, optional := range []string{"project_model", "project_type"} {
		a := resp.Schema.Attributes[optional]
		if !a.IsOptional() {
			t.Errorf("%s must be Optional", optional)
		}
		if a.IsRequired() {
			t.Errorf("%s must not be Required", optional)
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

func TestValidateWorkflowGroupConfig(t *testing.T) {
	tests := []struct {
		name              string
		typ               string
		hasModel, hasType bool
		wantOK            bool
	}{
		{"project with both filters", "project", true, true, true},
		{"project missing model", "project", false, true, false},
		{"project missing type", "project", true, false, false},
		{"project missing both", "project", false, false, false},
		{"product with no filters", "product", false, false, true},
		{"product with model forbidden", "product", true, false, false},
		{"product with type forbidden", "product", false, true, false},
		{"product with both forbidden", "product", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, detail := validateWorkflowGroupConfig(tt.typ, tt.hasModel, tt.hasType)
			gotOK := summary == ""
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v (summary=%q detail=%q), want %v", gotOK, summary, detail, tt.wantOK)
			}
			if !gotOK && detail == "" {
				t.Errorf("violation must carry a detail message")
			}
		})
	}
}

func wfg(id int64, model, projType string) *zentaoapi.WorkflowGroup {
	i := id
	m := model
	p := projType
	code := "c"
	return &zentaoapi.WorkflowGroup{ID: &i, ProjectModel: &m, ProjectType: &p, Code: &code}
}

func TestMatchProductWorkflowGroup(t *testing.T) {
	t.Run("single row returned", func(t *testing.T) {
		got, err := matchProductWorkflowGroup([]*zentaoapi.WorkflowGroup{wfg(1, "", "project")})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if deref(got.ID) != 1 {
			t.Fatalf("id = %d, want 1", deref(got.ID))
		}
	})
	t.Run("no rows is no-match", func(t *testing.T) {
		_, err := matchProductWorkflowGroup(nil)
		if !errors.Is(err, errNoWorkflowGroupMatch) {
			t.Fatalf("err = %v, want errNoWorkflowGroupMatch", err)
		}
	})
	t.Run("multiple rows is ambiguity", func(t *testing.T) {
		_, err := matchProductWorkflowGroup([]*zentaoapi.WorkflowGroup{wfg(1, "", "project"), wfg(2, "", "project")})
		if err == nil || errors.Is(err, errNoWorkflowGroupMatch) {
			t.Fatalf("err = %v, want ambiguity (not no-match)", err)
		}
	})
}

func TestMatchProjectWorkflowGroup(t *testing.T) {
	catalog := []*zentaoapi.WorkflowGroup{
		wfg(2, "scrum", "product"),
		wfg(3, "scrum", "project"),
		wfg(4, "waterfall", "product"),
	}
	t.Run("unique match", func(t *testing.T) {
		got, err := matchProjectWorkflowGroup(catalog, "scrum", "project")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if deref(got.ID) != 3 {
			t.Fatalf("id = %d, want 3", deref(got.ID))
		}
	})
	t.Run("no match", func(t *testing.T) {
		_, err := matchProjectWorkflowGroup(catalog, "kanban", "project")
		if !errors.Is(err, errNoWorkflowGroupMatch) {
			t.Fatalf("err = %v, want errNoWorkflowGroupMatch", err)
		}
	})
	t.Run("ambiguity", func(t *testing.T) {
		dup := append([]*zentaoapi.WorkflowGroup{}, catalog...)
		dup = append(dup, wfg(5, "scrum", "project"))
		_, err := matchProjectWorkflowGroup(dup, "scrum", "project")
		if err == nil || errors.Is(err, errNoWorkflowGroupMatch) {
			t.Fatalf("err = %v, want ambiguity (not no-match)", err)
		}
	})
}
