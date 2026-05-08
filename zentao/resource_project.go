package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*projectResource)(nil)
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
)

// Allowed enums per ZenTao v2 project API (probe-project-v2.md §2/§3).
var (
	projectModelEnum = []string{"scrum", "waterfall", "kanban", "agileplus", "waterfallplus", "cmmi"}
	projectACLEnum   = []string{"open", "private", "custom"}
)

type projectResource struct {
	client *zentaoapi.Client
}

type projectResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Model         types.String `tfsdk:"model"`
	Begin         types.String `tfsdk:"begin"`
	End           types.String `tfsdk:"end"`
	Program       types.Int64  `tfsdk:"program"`        // wire: Project.Parent
	Products      types.List   `tfsdk:"products"`       // wire: Project.Products
	WorkflowGroup types.Int64  `tfsdk:"workflow_group"` // wire: Project.WorkflowGroup
	ACL           types.String `tfsdk:"acl"`
	PM            types.String `tfsdk:"pm"`
	PO            types.String `tfsdk:"po"`
	QD            types.String `tfsdk:"qd"`
	RD            types.String `tfsdk:"rd"`
	Description   types.String `tfsdk:"description"`

	Code         types.String `tfsdk:"code"`
	Status       types.String `tfsdk:"status"`
	Lifetime     types.String `tfsdk:"lifetime"`
	OpenedBy     types.String `tfsdk:"opened_by"`
	OpenedDate   types.String `tfsdk:"opened_date"`
	LastEditedBy types.String `tfsdk:"last_edited_by"`
	RealBegan    types.String `tfsdk:"real_began"`
	RealEnd      types.String `tfsdk:"real_end"`
	Progress     types.String `tfsdk:"progress"`
	TeamCount    types.String `tfsdk:"team_count"`
	Budget       types.String `tfsdk:"budget"`
	BudgetUnit   types.String `tfsdk:"budget_unit"`
}

func NewProjectResource() resource.Resource { return &projectResource{} }

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	useStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao project (type=project) via the v2 RESTful API. " +
			"Beyond the documented v2 fields, ZenTao Max 8.x requires `products` (>=1) " +
			"and `workflow_group` on create; both are surfaced as first-class attributes. " +
			"Server-managed fields (audit columns, lifetime, progress, budget) are " +
			"exposed Computed-only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao project ID (stringified).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"name": schema.StringAttribute{
				Description: "Project display name.",
				Required:    true,
			},
			"model": schema.StringAttribute{
				Description: "Project execution model. One of: " + commaJoin(projectModelEnum) + ". " +
					"ZenTao does not support changing the model in-place, so this attribute " +
					"forces resource replacement on change.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{stringvalidator.OneOf(projectModelEnum...)},
			},
			"begin": schema.StringAttribute{
				Description: "Planned start date in YYYY-MM-DD format.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(dateRe, `must be YYYY-MM-DD`)},
			},
			"end": schema.StringAttribute{
				Description: "Planned end date in YYYY-MM-DD format.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(dateRe, `must be YYYY-MM-DD`)},
			},
			"program": schema.Int64Attribute{
				Description: "Parent program ID (mapped to ZenTao 'parent'). 0 means no " +
					"parent program; the server does not auto-assign one when omitted.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"products": schema.ListAttribute{
				Description: "Associated product IDs. ZenTao Max 8.x requires at least one " +
					"product on create (validator key 'productsBox'). The v2 GET endpoint " +
					"does not echo the product list back, so this attribute is preserved " +
					"from the prior plan/state to prevent spurious drift.",
				Required:    true,
				ElementType: types.Int64Type,
			},
			"workflow_group": schema.Int64Attribute{
				Description: "Workflow scheme id (mapped to ZenTao 'workflowGroup'). " +
					"Required by ZenTao Max 8.x on create. The server accepts any int; " +
					"there is no documented enum.",
				Required: true,
			},
			"acl": schema.StringAttribute{
				Description: "Access control. One of: " + commaJoin(projectACLEnum) + ". " +
					"Server-defaulted when unset (no static default here).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(projectACLEnum...)},
			},
			"pm": schema.StringAttribute{
				Description: "Project Manager username. ZenTao auto-assigns the calling " +
					"account when unset, so this stays Optional+Computed without a static " +
					"default to avoid 'inconsistent result after apply' drift.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"po": schema.StringAttribute{
				Description:   "Product Owner username. Server may auto-assign; Optional+Computed without static default.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"qd": schema.StringAttribute{
				Description:   "QA Lead username. Server may auto-assign; Optional+Computed without static default.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"rd": schema.StringAttribute{
				Description:   "Release Lead username. Server may auto-assign; Optional+Computed without static default.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"description": schema.StringAttribute{
				Description:   "Optional description (mapped to ZenTao 'desc'). Empty string when unset.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				PlanModifiers: useStateForString,
			},

			// Server-managed read-only attributes.
			"code": schema.StringAttribute{
				Description:   "Project short code (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"status": schema.StringAttribute{
				Description:   "Server-managed project status (e.g. \"wait\", \"doing\", \"closed\").",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"lifetime": schema.StringAttribute{
				Description:   "Project lifetime classification (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"opened_by": schema.StringAttribute{
				Description:   "Creator username (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"opened_date": schema.StringAttribute{
				Description:   "Creation timestamp (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"last_edited_by": schema.StringAttribute{
				Description:   "Last editor username (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"real_began": schema.StringAttribute{
				Description:   "Actual start date (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"real_end": schema.StringAttribute{
				Description:   "Actual end date (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"progress": schema.StringAttribute{
				Description:   "Completion progress percentage (server-managed; string per ZenTao API).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"team_count": schema.StringAttribute{
				Description:   "Total team members (server-managed; string per ZenTao API).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"budget": schema.StringAttribute{
				Description:   "Project budget amount (server-managed; string per ZenTao API).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"budget_unit": schema.StringAttribute{
				Description:   "Currency unit of the budget (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.client = client
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiInput, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateProject(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Create project failed", err.Error())
		return
	}
	fetched, err := r.client.GetProject(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Re-fetch after create failed", err.Error())
		return
	}
	// GetProject does not decode the products list (the v2 GET response
	// uses hasProduct/multiple flags and the actual list is on a
	// separate endpoint we don't consume). Splice the planned products
	// onto the fetched object so the resulting state matches what the
	// user requested rather than collapsing to an empty list.
	fetched.Products = apiInput.Products
	state, diags := projectFromAPI(ctx, fetched)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetProject(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read project failed", err.Error())
		return
	}
	state, diags := projectFromAPI(ctx, fetched)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// GetProject does not return the products list, so we preserve the
	// prior state's list verbatim. This avoids spurious drift on every
	// plan when nothing has actually changed product-side.
	state.Products = prior.Products
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	apiInput, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiInput.ID = id
	updated, err := r.client.UpdateProject(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update project failed", err.Error())
		return
	}
	// As in Create: the re-fetch performed inside UpdateProject does not
	// repopulate Products, so splice the planned list back in before
	// projecting to state.
	updated.Products = apiInput.Products
	state, diags := projectFromAPI(ctx, updated)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteProject(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete project failed", err.Error())
		return
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toAPI projects a Terraform plan into the API shape sent over the wire.
// Server-managed read-only fields stay zero so json:"-" tags strip them.
func (m *projectResourceModel) toAPI(ctx context.Context) (*zentaoapi.Project, diag.Diagnostics) {
	var products []int64
	diags := m.Products.ElementsAs(ctx, &products, true)
	intProducts := make([]int, 0, len(products))
	for _, v := range products {
		intProducts = append(intProducts, int(v))
	}
	return &zentaoapi.Project{
		Name:          m.Name.ValueString(),
		Model:         m.Model.ValueString(),
		Begin:         m.Begin.ValueString(),
		End:           m.End.ValueString(),
		Parent:        int(m.Program.ValueInt64()),
		Products:      intProducts,
		WorkflowGroup: int(m.WorkflowGroup.ValueInt64()),
		ACL:           m.ACL.ValueString(),
		PM:            m.PM.ValueString(),
		PO:            m.PO.ValueString(),
		QD:            m.QD.ValueString(),
		RD:            m.RD.ValueString(),
		Description:   m.Description.ValueString(),
	}, diags
}

func projectFromAPI(ctx context.Context, p *zentaoapi.Project) (projectResourceModel, diag.Diagnostics) {
	src := p.Products
	if src == nil {
		src = []int{}
	}
	productsList, diags := types.ListValueFrom(ctx, types.Int64Type, src)
	return projectResourceModel{
		ID:            types.StringValue(strconv.Itoa(p.ID)),
		Name:          types.StringValue(p.Name),
		Model:         types.StringValue(p.Model),
		Begin:         types.StringValue(p.Begin),
		End:           types.StringValue(p.End),
		Program:       types.Int64Value(int64(p.Parent)),
		Products:      productsList,
		WorkflowGroup: types.Int64Value(int64(p.WorkflowGroup)),
		ACL:           types.StringValue(p.ACL),
		PM:            types.StringValue(p.PM),
		PO:            types.StringValue(p.PO),
		QD:            types.StringValue(p.QD),
		RD:            types.StringValue(p.RD),
		Description:   types.StringValue(p.Description),
		Code:          types.StringValue(p.Code),
		Status:        types.StringValue(p.Status),
		Lifetime:      types.StringValue(p.Lifetime),
		OpenedBy:      types.StringValue(p.OpenedBy),
		OpenedDate:    types.StringValue(p.OpenedDate),
		LastEditedBy:  types.StringValue(p.LastEditedBy),
		RealBegan:     types.StringValue(p.RealBegan),
		RealEnd:       types.StringValue(p.RealEnd),
		Progress:      types.StringValue(p.Progress),
		TeamCount:     types.StringValue(p.TeamCount),
		Budget:        types.StringValue(p.Budget),
		BudgetUnit:    types.StringValue(p.BudgetUnit),
	}, diags
}
