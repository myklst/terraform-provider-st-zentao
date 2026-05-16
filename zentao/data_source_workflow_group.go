package zentao

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ datasource.DataSource              = (*workflowGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*workflowGroupDataSource)(nil)
)

var workflowProjectTypeEnum = []string{"product", "project"}

type workflowGroupDataSource struct {
	client *zentaoapi.Client
}

type workflowGroupDataSourceModel struct {
	ProjectModel types.String `tfsdk:"project_model"`
	ProjectType  types.String `tfsdk:"project_type"`

	ID   types.Int64  `tfsdk:"id"`
	Code types.String `tfsdk:"code"`
	Name types.String `tfsdk:"name"`
}

func NewWorkflowGroupDataSource() datasource.DataSource { return &workflowGroupDataSource{} }

func (d *workflowGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow_group"
}

func (d *workflowGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a ZenTao workflow group by (project_model, project_type). " +
			"Use the resulting `id` to populate `st-zentao_project.workflow_group` without hardcoding ZenTao instance-specific magic numbers.",
		Attributes: map[string]schema.Attribute{
			"project_model": schema.StringAttribute{
				Description: "Project methodology. One of: " + commaJoin(projectModelEnum) + ".",
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf(projectModelEnum...)},
			},
			"project_type": schema.StringAttribute{
				Description: "Whether the workflow group is product-typed (`product`) or project-typed (`project`).",
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf(workflowProjectTypeEnum...)},
			},
			"id": schema.Int64Attribute{
				Description: "Numeric workflow group id, used as the value for `workflow_group` on `st-zentao_project`.",
				Computed:    true,
			},
			"code": schema.StringAttribute{
				Description: "Stable code identifier (e.g. `scrumproduct`).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Localised display name.",
				Computed:    true,
			},
		},
	}
}

func (d *workflowGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*zentaoapi.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("got %T, want *zentaoapi.Client", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *workflowGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg workflowGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	wfg, err := d.client.FindWorkflowGroup(ctx, cfg.ProjectModel.ValueString(), cfg.ProjectType.ValueString())
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError(
			"No matching workflow group",
			fmt.Sprintf("ZenTao has no workflow group with project_model=%q project_type=%q. "+
				"Ask your ZenTao admin which workflow groups exist (see Admin → Workflow → Project workflows).",
				cfg.ProjectModel.ValueString(), cfg.ProjectType.ValueString()),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Workflow group lookup failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workflowGroupDataSourceModel{
		ProjectModel: cfg.ProjectModel,
		ProjectType:  cfg.ProjectType,
		ID:           types.Int64Value(deref(wfg.ID)),
		Code:         types.StringValue(deref(wfg.Code)),
		Name:         types.StringValue(deref(wfg.Name)),
	})...)
}
