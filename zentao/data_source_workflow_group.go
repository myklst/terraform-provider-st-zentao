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

const (
	workflowGroupTypeProduct = "product"
	workflowGroupTypeProject = "project"
)

// workflowGroupTypeEnum selects which catalog/endpoint to query: the 产品流程
// (`product`) or 项目流程 (`project`) catalog. Distinct from project_type.
var workflowGroupTypeEnum = []string{workflowGroupTypeProduct, workflowGroupTypeProject}

var workflowProjectTypeEnum = []string{"product", "project"}

// errNoWorkflowGroupMatch is the sentinel returned by the match helpers when
// no row satisfies the lookup, letting Read surface a friendly diagnostic.
var errNoWorkflowGroupMatch = errors.New("no matching workflow group")

type workflowGroupDataSource struct {
	client *zentaoapi.Client
}

type workflowGroupDataSourceModel struct {
	Type         types.String `tfsdk:"type"`
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
		Description: "Look up a ZenTao workflow group and resolve its numeric `id` for use as " +
			"`st-zentao_project.workflow_group`, without hardcoding instance-specific magic numbers. " +
			"Set `type` to choose the catalog: `product` returns the single product flow; " +
			"`project` requires `project_model` and `project_type` to pick one project flow.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Description: "Workflow catalog to query. One of: " + commaJoin(workflowGroupTypeEnum) + ".",
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf(workflowGroupTypeEnum...)},
			},
			"project_model": schema.StringAttribute{
				Description: "Project methodology. One of: " + commaJoin(projectModelEnum) + ". " +
					"Required when `type` is `project`; must be omitted when `type` is `product`.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.OneOf(projectModelEnum...)},
			},
			"project_type": schema.StringAttribute{
				Description: "Whether the project flow is product-typed (`product`) or project-typed (`project`). " +
					"Required when `type` is `project`; must be omitted when `type` is `product`.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.OneOf(workflowProjectTypeEnum...)},
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

// validateWorkflowGroupConfig enforces the conditional contract between `type`
// and the (project_model, project_type) filters. It returns empty strings when
// the config is valid, or a (summary, detail) diagnostic pair otherwise.
//
//   - type=project requires both filters (the project catalog is subdivided by
//     projectModel × projectType, so an unfiltered lookup is ambiguous).
//   - type=product forbids both filters (the product catalog has a single
//     default row whose projectModel is empty — there is nothing to filter on).
func validateWorkflowGroupConfig(typ string, hasModel, hasType bool) (summary, detail string) {
	switch typ {
	case workflowGroupTypeProject:
		if !hasModel || !hasType {
			return "Missing required attributes",
				"`project_model` and `project_type` are both required when `type` is `project`."
		}
	case workflowGroupTypeProduct:
		if hasModel || hasType {
			return "Conflicting attributes",
				"`project_model` and `project_type` must be omitted when `type` is `product`; " +
					"the product catalog has a single default flow with no methodology to filter on."
		}
	}
	return "", ""
}

// matchProductWorkflowGroup picks the lone product-flow row. The factory
// catalog has exactly one; if an admin added more there is no narrowing knob,
// so ambiguity is surfaced rather than silently picking.
func matchProductWorkflowGroup(groups []*zentaoapi.WorkflowGroup) (*zentaoapi.WorkflowGroup, error) {
	switch len(groups) {
	case 0:
		return nil, errNoWorkflowGroupMatch
	case 1:
		return groups[0], nil
	default:
		return nil, fmt.Errorf("%d product workflow groups exist (ids=%v) — the product catalog is expected to hold a single default flow; ask your ZenTao admin to remove the duplicates", len(groups), workflowGroupIDs(groups))
	}
}

// matchProjectWorkflowGroup filters the project catalog by (projectModel,
// projectType). Zero matches yields errNoWorkflowGroupMatch; multiple matches
// (admin-created duplicates) yields an ambiguity error.
func matchProjectWorkflowGroup(groups []*zentaoapi.WorkflowGroup, projectModel, projectType string) (*zentaoapi.WorkflowGroup, error) {
	var matches []*zentaoapi.WorkflowGroup
	for _, g := range groups {
		if deref(g.ProjectModel) == projectModel && deref(g.ProjectType) == projectType {
			matches = append(matches, g)
		}
	}
	switch len(matches) {
	case 0:
		return nil, errNoWorkflowGroupMatch
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("%d workflow groups match project_model=%q project_type=%q (ids=%v) — your ZenTao admin has duplicates, please disambiguate",
			len(matches), projectModel, projectType, workflowGroupIDs(matches))
	}
}

func workflowGroupIDs(groups []*zentaoapi.WorkflowGroup) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, deref(g.ID))
	}
	return ids
}

func (d *workflowGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg workflowGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	typ := cfg.Type.ValueString()
	if summary, detail := validateWorkflowGroupConfig(typ, !cfg.ProjectModel.IsNull(), !cfg.ProjectType.IsNull()); summary != "" {
		resp.Diagnostics.AddError(summary, detail)
		return
	}

	var (
		wfg *zentaoapi.WorkflowGroup
		err error
	)
	switch typ {
	case workflowGroupTypeProduct:
		groups, listErr := d.client.ListProductWorkflowGroups(ctx)
		if listErr != nil {
			resp.Diagnostics.AddError("Workflow group lookup failed", listErr.Error())
			return
		}
		wfg, err = matchProductWorkflowGroup(groups)
	case workflowGroupTypeProject:
		groups, listErr := d.client.ListProjectWorkflowGroups(ctx)
		if listErr != nil {
			resp.Diagnostics.AddError("Workflow group lookup failed", listErr.Error())
			return
		}
		wfg, err = matchProjectWorkflowGroup(groups, cfg.ProjectModel.ValueString(), cfg.ProjectType.ValueString())
	}

	if errors.Is(err, errNoWorkflowGroupMatch) {
		resp.Diagnostics.AddError(
			"No matching workflow group",
			fmt.Sprintf("ZenTao has no %q workflow group matching project_model=%q project_type=%q. "+
				"Ask your ZenTao admin which workflow groups exist (see Admin → Workflow).",
				typ, cfg.ProjectModel.ValueString(), cfg.ProjectType.ValueString()),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Workflow group lookup failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, workflowGroupDataSourceModel{
		Type:         cfg.Type,
		ProjectModel: cfg.ProjectModel,
		ProjectType:  cfg.ProjectType,
		ID:           types.Int64Value(deref(wfg.ID)),
		Code:         types.StringValue(deref(wfg.Code)),
		Name:         types.StringValue(deref(wfg.Name)),
	})...)
}
