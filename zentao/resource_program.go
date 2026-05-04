package zentao

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
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

// dateRe enforces YYYY-MM-DD as required by the v2 begin/end fields.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

type programResource struct {
	client *zentaoapi.Client
}

type programResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Code types.String `tfsdk:"code"`

	Name        types.String `tfsdk:"name"`
	Begin       types.String `tfsdk:"begin"`
	End         types.String `tfsdk:"end"`
	PM          types.String `tfsdk:"pm"`
	Description types.String `tfsdk:"description"`

	Status     types.String `tfsdk:"status"`
	Parent     types.Int64  `tfsdk:"parent"`
	Type       types.String `tfsdk:"type"`
	Category   types.String `tfsdk:"category"`
	ACL        types.String `tfsdk:"acl"`
	PO         types.String `tfsdk:"po"`
	QD         types.String `tfsdk:"qd"`
	RD         types.String `tfsdk:"rd"`
	Budget     types.String `tfsdk:"budget"`
	BudgetUnit types.String `tfsdk:"budget_unit"`
	OpenedBy   types.String `tfsdk:"opened_by"`
	OpenedDate types.String `tfsdk:"opened_date"`
	RealBegan  types.String `tfsdk:"real_began"`
	RealEnd    types.String `tfsdk:"real_end"`
	Progress   types.String `tfsdk:"progress"`
	TeamCount  types.String `tfsdk:"team_count"`
}

func NewProgramResource() resource.Resource { return &programResource{} }

func (r *programResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program"
}

func (r *programResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao program (project portfolio) via the v2 RESTful API. " +
			"The v2 create/update endpoints accept only name/begin/end/PM/desc; everything " +
			"else (status, audit columns, role assignments, budget) is server-managed and " +
			"exposed as Computed read-only attributes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao program ID (stringified).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"code": schema.StringAttribute{
				Description:   "Program short code (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"name": schema.StringAttribute{
				Description: "Program display name.",
				Required:    true,
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
			"pm": schema.StringAttribute{
				Description: "Program Manager username. ZenTao auto-assigns the calling " +
					"account when unset, so this stays Optional+Computed without a static " +
					"default to avoid 'inconsistent result after apply' drift.",
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
			"status": schema.StringAttribute{
				Description:   "Server-managed program status (e.g. \"wait\", \"doing\", \"closed\").",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"parent": schema.Int64Attribute{
				Description: "Parent program ID (0 if top-level).",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description:   "Program type (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"category": schema.StringAttribute{
				Description:   "Program category (server-managed; e.g. \"rnd\").",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"acl": schema.StringAttribute{
				Description:   "Access control level (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"po": schema.StringAttribute{
				Description:   "Product Owner username (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"qd": schema.StringAttribute{
				Description:   "QA Lead username (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"rd": schema.StringAttribute{
				Description:   "Release Lead username (server-managed).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"budget": schema.StringAttribute{
				Description:   "Program budget amount (server-managed; string per ZenTao API).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"budget_unit": schema.StringAttribute{
				Description:   "Currency unit of the budget (server-managed).",
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
	fetched, err := r.client.GetProgram(ctx, created.ID)
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
	id, err := strconv.Atoi(prior.ID.ValueString())
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
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	apiInput := plan.toAPI()
	apiInput.ID = id
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
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteProgram(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete program failed", err.Error())
		return
	}
}

func (r *programResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *programResourceModel) toAPI() *zentaoapi.Program {
	return &zentaoapi.Program{
		Name:        m.Name.ValueString(),
		Begin:       m.Begin.ValueString(),
		End:         m.End.ValueString(),
		PM:          m.PM.ValueString(),
		Description: m.Description.ValueString(),
	}
}

func programFromAPI(p *zentaoapi.Program) programResourceModel {
	return programResourceModel{
		ID:          types.StringValue(strconv.Itoa(p.ID)),
		Code:        types.StringValue(p.Code),
		Name:        types.StringValue(p.Name),
		Begin:       types.StringValue(p.Begin),
		End:         types.StringValue(p.End),
		PM:          types.StringValue(p.PM),
		Description: types.StringValue(p.Description),
		Status:      types.StringValue(p.Status),
		Parent:      types.Int64Value(int64(p.Parent)),
		Type:        types.StringValue(p.Type),
		Category:    types.StringValue(p.Category),
		ACL:         types.StringValue(p.ACL),
		PO:          types.StringValue(p.PO),
		QD:          types.StringValue(p.QD),
		RD:          types.StringValue(p.RD),
		Budget:      types.StringValue(p.Budget),
		BudgetUnit:  types.StringValue(p.BudgetUnit),
		OpenedBy:    types.StringValue(p.OpenedBy),
		OpenedDate:  types.StringValue(p.OpenedDate),
		RealBegan:   types.StringValue(p.RealBegan),
		RealEnd:     types.StringValue(p.RealEnd),
		Progress:    types.StringValue(p.Progress),
		TeamCount:   types.StringValue(p.TeamCount),
	}
}

// keep the diag import live for future helpers (e.g., when adding list attrs).
var _ diag.Diagnostics
