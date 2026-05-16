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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
	Program       types.Int64  `tfsdk:"program"`
	Products      types.List   `tfsdk:"products"`
	WorkflowGroup types.Int64  `tfsdk:"workflow_group"`
	Multiple      types.Bool   `tfsdk:"multiple"`
	ACL           types.String `tfsdk:"acl"`
	PM            types.String `tfsdk:"pm"`
	PO            types.String `tfsdk:"po"`
	QD            types.String `tfsdk:"qd"`
	RD            types.String `tfsdk:"rd"`
	Desc          types.String `tfsdk:"desc"`
	Status        types.String `tfsdk:"status"`
}

func NewProjectResource() resource.Resource { return &projectResource{} }

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	useStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao project ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"name": schema.StringAttribute{
				Description: "Project display name.",
				Required:    true,
			},
			"model": schema.StringAttribute{
				Description:   "Project execution model. One of: " + commaJoin(projectModelEnum) + ". Changing this forces resource replacement.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:    []validator.String{stringvalidator.OneOf(projectModelEnum...)},
			},
			"begin": schema.StringAttribute{
				Description: "Planned start date (YYYY-MM-DD).",
				Required:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(dateRe, `must be YYYY-MM-DD`)},
			},
			"end": schema.StringAttribute{
				Description: "Planned end date (YYYY-MM-DD).",
				Required:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(dateRe, `must be YYYY-MM-DD`)},
			},
			"program": schema.Int64Attribute{
				Description:   "Parent program id (0 = no parent program).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"products": schema.ListAttribute{
				Description: "Associated product IDs (at least one required).",
				Required:    true,
				ElementType: types.Int64Type,
			},
			"workflow_group": schema.Int64Attribute{
				Description:   "Workflow scheme id. Changing this forces resource replacement.",
				Required:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"multiple": schema.BoolAttribute{
				Description: "Whether iterations (sprints) are enabled. Create-only; changing this forces resource replacement.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"acl": schema.StringAttribute{
				Description:   "Access control. One of: " + commaJoin(projectACLEnum) + ".",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf(projectACLEnum...)},
				PlanModifiers: useStateForString,
			},
			"pm": schema.StringAttribute{
				Description:   "Project Manager username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"po": schema.StringAttribute{
				Description:   "Product Owner username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"qd": schema.StringAttribute{
				Description:   "QA Lead username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"rd": schema.StringAttribute{
				Description:   "Release Lead username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"desc": schema.StringAttribute{
				Description:   "Description.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"status": schema.StringAttribute{
				Description: "Lifecycle status (wait / doing / suspended / closed). Server-managed; changes outside of Terraform are surfaced on next read.",
				Computed:    true,
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
	state, diags := projectFromAPI(ctx, created)
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
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
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
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	apiInput, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiInput.ID = &id
	updated, err := r.client.UpdateProject(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update project failed", err.Error())
		return
	}
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
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
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

// toAPI converts plan/state into the pointer-typed API Project. Nil
// pointers signal "field absent from input"; the API wrapper's
// mergeProjectBaseline reads that as "preserve baseline" on edits.
func (m *projectResourceModel) toAPI(ctx context.Context) (*zentaoapi.Project, diag.Diagnostics) {
	var diags diag.Diagnostics
	products, pdiags := optInt64List(ctx, m.Products)
	diags.Append(pdiags...)
	return &zentaoapi.Project{
		Name:          optString(m.Name),
		Model:         optString(m.Model),
		Begin:         optString(m.Begin),
		End:           optString(m.End),
		Parent:        optInt64(m.Program),
		Products:      products,
		WorkflowGroup: optInt64(m.WorkflowGroup),
		Multiple:      optBool(m.Multiple),
		ACL:           optString(m.ACL),
		PM:            optString(m.PM),
		PO:            optString(m.PO),
		QD:            optString(m.QD),
		RD:            optString(m.RD),
		Desc:          optString(m.Desc),
	}, diags
}

func projectFromAPI(ctx context.Context, p *zentaoapi.Project) (projectResourceModel, diag.Diagnostics) {
	products := deref(p.Products)
	if products == nil {
		products = []int64{}
	}
	productsList, diags := types.ListValueFrom(ctx, types.Int64Type, products)
	return projectResourceModel{
		ID:            types.StringValue(strconv.FormatInt(deref(p.ID), 10)),
		Name:          types.StringValue(deref(p.Name)),
		Model:         types.StringValue(deref(p.Model)),
		Begin:         types.StringValue(deref(p.Begin)),
		End:           types.StringValue(deref(p.End)),
		Program:       types.Int64Value(deref(p.Parent)),
		Products:      productsList,
		WorkflowGroup: types.Int64Value(deref(p.WorkflowGroup)),
		Multiple:      types.BoolValue(deref(p.Multiple)),
		ACL:           types.StringValue(deref(p.ACL)),
		PM:            types.StringValue(deref(p.PM)),
		PO:            types.StringValue(deref(p.PO)),
		QD:            types.StringValue(deref(p.QD)),
		RD:            types.StringValue(deref(p.RD)),
		Desc:          types.StringValue(deref(p.Desc)),
		Status: types.StringValue(deref(p.Status)),
	}, diags
}
