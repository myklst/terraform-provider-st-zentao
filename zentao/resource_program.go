package zentao

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*programResource)(nil)
	_ resource.ResourceWithConfigure   = (*programResource)(nil)
	_ resource.ResourceWithImportState = (*programResource)(nil)
)

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var programACLEnum = []string{"open", "private", "custom"}

type programResource struct {
	client *zentaoapi.Client
}

type programResourceModel struct {
	ID types.String `tfsdk:"id"`

	Parent     types.Int64  `tfsdk:"parent"`
	Name       types.String `tfsdk:"name"`
	PM         types.String `tfsdk:"pm"`
	Budget     types.String `tfsdk:"budget"`
	BudgetUnit types.String `tfsdk:"budget_unit"`
	Begin      types.String `tfsdk:"begin"`
	End        types.String `tfsdk:"end"`
	Desc       types.String `tfsdk:"desc"`
	Status     types.String `tfsdk:"status"`
	ACL        types.String `tfsdk:"acl"`
	Whitelist  types.String `tfsdk:"whitelist"`
}

func NewProgramResource() resource.Resource { return &programResource{} }

func (r *programResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program"
}

func (r *programResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	pmUseStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	pmUseStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	pmStringRequiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	// pmIntRequiresReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao program (project portfolio).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao program ID.",
				Computed:      true,
				PlanModifiers: pmStringRequiresReplace,
			},
			"parent": schema.Int64Attribute{
				Description:   "Parent program id (0 = top-level). Read-only; set via `st-zentao_program_parent_attachment`.",
				Computed:      true,
				PlanModifiers: pmUseStateForInt,
			},
			"name": schema.StringAttribute{
				Description: "Program display name.",
				Required:    true,
			},
			"pm": schema.StringAttribute{
				Description:   "Project Manager username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: pmUseStateForString,
			},
			"budget": schema.StringAttribute{
				Description:   "Program budget amount.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: pmUseStateForString,
			},
			"budget_unit": schema.StringAttribute{
				Description:   "Currency unit of the budget.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: pmUseStateForString,
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
			"desc": schema.StringAttribute{
				Description: "Description.",
				Optional:    true,
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status.",
				Computed:    true,
			},
			"acl": schema.StringAttribute{
				Description: "Access control. One of: " + commaJoin(programACLEnum) + ".",
				Optional:    true,
				Computed:    true,
				Validators:  []validator.String{stringvalidator.OneOf(programACLEnum...)},
			},
			"whitelist": schema.StringAttribute{
				Description: "Comma-joined account list when `acl = \"custom\"`.",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

func (r *programResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *programResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan programResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateProgram(ctx, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Create program failed", err.Error())
		return
	}
	if created.ID == nil {
		resp.Diagnostics.AddError("Create program failed", "server returned no id")
		return
	}
	fetched, err := r.client.GetProgram(ctx, *created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Re-fetch after create failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, programFromAPI(fetched))...)
}

func (r *programResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior programResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetProgram(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read program failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, programFromAPI(fetched))...)
}

func (r *programResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior programResourceModel
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
	apiInput := plan.toAPI()
	apiInput.ID = &id
	updated, err := r.client.UpdateProgram(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update program failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, programFromAPI(updated))...)
}

func (r *programResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior programResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteProgram(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete program failed", err.Error())
		return
	}
	// Update state
	fetched, err := r.client.GetProgram(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Re-fetch after delete failed "+strconv.FormatInt(id, 10), err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, programFromAPI(fetched))...)
}

func (r *programResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *programResourceModel) toAPI() *zentaoapi.Program {
	// Parent is intentionally omitted — the attachment resource owns it.
	// UpdateProgram's M-Z merge preserves the column when input.Parent is nil.
	return &zentaoapi.Program{
		Name:       optString(m.Name),
		PM:         optString(m.PM),
		Budget:     optString(m.Budget),
		BudgetUnit: optString(m.BudgetUnit),
		Begin:      optString(m.Begin),
		End:        optString(m.End),
		Desc:       optString(m.Desc),
		Status:     optString(m.Status),
		ACL:        optString(m.ACL),
		Whitelist:  optString(m.Whitelist),
	}
}

func programFromAPI(p *zentaoapi.Program) programResourceModel {
	return programResourceModel{
		ID:         types.StringValue(strconv.FormatInt(derefInt64(p.ID), 10)),
		Parent:     types.Int64Value(derefInt64(p.Parent)),
		Name:       types.StringValue(derefString(p.Name)),
		PM:         types.StringValue(derefString(p.PM)),
		Budget:     types.StringValue(derefString(p.Budget)),
		BudgetUnit: types.StringValue(derefString(p.BudgetUnit)),
		Begin:      types.StringValue(derefString(p.Begin)),
		End:        types.StringValue(derefString(p.End)),
		Desc:       types.StringValue(derefString(p.Desc)),
		Status:     types.StringValue(derefString(p.Status)),
		ACL:        types.StringValue(derefString(p.ACL)),
		Whitelist:  types.StringValue(derefString(p.Whitelist)),
	}
}
