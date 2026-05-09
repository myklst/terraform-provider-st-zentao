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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
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

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var programACLEnum = []string{"open", "private", "custom"}

type programResource struct {
	client *zentaoapi.Client
}

type programResourceModel struct {
	ID types.String `tfsdk:"id"`

	Name       types.String `tfsdk:"name"`
	Begin      types.String `tfsdk:"begin"`
	End        types.String `tfsdk:"end"`
	Parent     types.Int64  `tfsdk:"parent"`
	PM         types.String `tfsdk:"pm"`
	Desc       types.String `tfsdk:"desc"`
	ACL        types.String `tfsdk:"acl"`
	Budget     types.String `tfsdk:"budget"`
	BudgetUnit types.String `tfsdk:"budget_unit"`
	Whitelist  types.String `tfsdk:"whitelist"`

	Code           types.String `tfsdk:"code"`
	Status         types.String `tfsdk:"status"`
	Type           types.String `tfsdk:"type"`
	Category       types.String `tfsdk:"category"`
	Lifetime       types.String `tfsdk:"lifetime"`
	Vision         types.String `tfsdk:"vision"`
	Attribute      types.String `tfsdk:"attribute"`
	Model          types.String `tfsdk:"model"`
	ProgramPath    types.String `tfsdk:"program_path"`
	Grade          types.Int64  `tfsdk:"grade"`
	Multiple       types.Bool   `tfsdk:"multiple"`
	Parallel       types.Bool   `tfsdk:"parallel"`
	HasProduct     types.Bool   `tfsdk:"has_product"`
	WorkflowGroup  types.Int64  `tfsdk:"workflow_group"`
	StoryType      types.String `tfsdk:"story_type"`
	Days           types.Int64  `tfsdk:"days"`
	FirstEnd       types.String `tfsdk:"first_end"`
	OpenedBy       types.String `tfsdk:"opened_by"`
	OpenedDate     types.String `tfsdk:"opened_date"`
	LastEditedBy   types.String `tfsdk:"last_edited_by"`
	LastEditedDate types.String `tfsdk:"last_edited_date"`
	RealBegan      types.String `tfsdk:"real_began"`
	RealEnd        types.String `tfsdk:"real_end"`
	ClosedBy       types.String `tfsdk:"closed_by"`
	ClosedDate     types.String `tfsdk:"closed_date"`
	ClosedReason   types.String `tfsdk:"closed_reason"`
	CanceledBy     types.String `tfsdk:"canceled_by"`
	CanceledDate   types.String `tfsdk:"canceled_date"`
	SuspendedDate  types.String `tfsdk:"suspended_date"`
	PO             types.String `tfsdk:"po"`
	QD             types.String `tfsdk:"qd"`
	RD             types.String `tfsdk:"rd"`
	Team           types.String `tfsdk:"team"`
	Progress       types.String `tfsdk:"progress"`
	Percent        types.String `tfsdk:"percent"`
	Estimate       types.String `tfsdk:"estimate"`
	Consumed       types.String `tfsdk:"consumed"`
	Left           types.String `tfsdk:"left"`
	TeamCount      types.Int64  `tfsdk:"team_count"`
}

func NewProgramResource() resource.Resource { return &programResource{} }

func (r *programResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program"
}

func (r *programResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	useStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	useStateForBool := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao program (project portfolio).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao program ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"name": schema.StringAttribute{
				Description: "Program display name.",
				Required:    true,
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
			"parent": schema.Int64Attribute{
				Description:   "Parent program id (0 = top-level). Read-only; set via `st-zentao_program_parent_attachment`.",
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"pm": schema.StringAttribute{
				Description:   "Project Manager username.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"desc": schema.StringAttribute{
				Description:   "Description.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				PlanModifiers: useStateForString,
			},
			"acl": schema.StringAttribute{
				Description:   "Access control. One of: " + commaJoin(programACLEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(programACLEnum...)},
			},
			"budget": schema.StringAttribute{
				Description:   "Program budget amount.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"budget_unit": schema.StringAttribute{
				Description:   "Currency unit of the budget.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"whitelist": schema.StringAttribute{
				Description:   "Comma-joined account list when `acl = \"custom\"`.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
			},

			"code":             schema.StringAttribute{Description: "Program short code.", Computed: true, PlanModifiers: useStateForString},
			"status":           schema.StringAttribute{Description: "Program status.", Computed: true, PlanModifiers: useStateForString},
			"type":             schema.StringAttribute{Description: "Program type.", Computed: true, PlanModifiers: useStateForString},
			"category":         schema.StringAttribute{Description: "Program category.", Computed: true, PlanModifiers: useStateForString},
			"lifetime":         schema.StringAttribute{Description: "Program lifetime classification.", Computed: true, PlanModifiers: useStateForString},
			"vision":           schema.StringAttribute{Description: "Vision (`rnd` / `lite`).", Computed: true, PlanModifiers: useStateForString},
			"attribute":        schema.StringAttribute{Description: "Attribute.", Computed: true, PlanModifiers: useStateForString},
			"model":            schema.StringAttribute{Description: "Methodology marker.", Computed: true, PlanModifiers: useStateForString},
			"program_path":     schema.StringAttribute{Description: "Hierarchy path.", Computed: true, PlanModifiers: useStateForString},
			"grade":            schema.Int64Attribute{Description: "Depth in program hierarchy.", Computed: true, PlanModifiers: useStateForInt},
			"multiple":         schema.BoolAttribute{Description: "Whether iterations / sub-projects are enabled.", Computed: true, PlanModifiers: useStateForBool},
			"parallel":         schema.BoolAttribute{Description: "Parallel execution flag.", Computed: true, PlanModifiers: useStateForBool},
			"has_product":      schema.BoolAttribute{Description: "Whether the program has any associated products.", Computed: true, PlanModifiers: useStateForBool},
			"workflow_group":   schema.Int64Attribute{Description: "Workflow scheme id.", Computed: true, PlanModifiers: useStateForInt},
			"story_type":       schema.StringAttribute{Description: "Story type marker.", Computed: true, PlanModifiers: useStateForString},
			"days":             schema.Int64Attribute{Description: "Working-day count.", Computed: true, PlanModifiers: useStateForInt},
			"first_end":        schema.StringAttribute{Description: "First planned end date.", Computed: true, PlanModifiers: useStateForString},
			"opened_by":        schema.StringAttribute{Description: "Creator username.", Computed: true, PlanModifiers: useStateForString},
			"opened_date":      schema.StringAttribute{Description: "Creation timestamp.", Computed: true, PlanModifiers: useStateForString},
			"last_edited_by":   schema.StringAttribute{Description: "Last-editor username.", Computed: true},
			"last_edited_date": schema.StringAttribute{Description: "Timestamp of the last edit.", Computed: true},
			"real_began":       schema.StringAttribute{Description: "Actual start date.", Computed: true, PlanModifiers: useStateForString},
			"real_end":         schema.StringAttribute{Description: "Actual end date.", Computed: true, PlanModifiers: useStateForString},
			"closed_by":        schema.StringAttribute{Description: "User who closed the program.", Computed: true, PlanModifiers: useStateForString},
			"closed_date":      schema.StringAttribute{Description: "When the program was closed.", Computed: true, PlanModifiers: useStateForString},
			"closed_reason":    schema.StringAttribute{Description: "Reason recorded at close time.", Computed: true, PlanModifiers: useStateForString},
			"canceled_by":      schema.StringAttribute{Description: "User who canceled the program.", Computed: true, PlanModifiers: useStateForString},
			"canceled_date":    schema.StringAttribute{Description: "When the program was canceled.", Computed: true, PlanModifiers: useStateForString},
			"suspended_date":   schema.StringAttribute{Description: "When the program was suspended.", Computed: true, PlanModifiers: useStateForString},
			"po":               schema.StringAttribute{Description: "Product Owner username.", Computed: true, PlanModifiers: useStateForString},
			"qd":               schema.StringAttribute{Description: "QA Lead username.", Computed: true, PlanModifiers: useStateForString},
			"rd":               schema.StringAttribute{Description: "Release Lead username.", Computed: true, PlanModifiers: useStateForString},
			"team":             schema.StringAttribute{Description: "Team designator.", Computed: true, PlanModifiers: useStateForString},
			"progress":         schema.StringAttribute{Description: "Completion progress percentage.", Computed: true},
			"percent":          schema.StringAttribute{Description: "Auxiliary completion percent.", Computed: true},
			"estimate":         schema.StringAttribute{Description: "Estimated effort.", Computed: true},
			"consumed":         schema.StringAttribute{Description: "Consumed effort.", Computed: true},
			"left":             schema.StringAttribute{Description: "Remaining effort.", Computed: true},
			"team_count":       schema.Int64Attribute{Description: "Total team members.", Computed: true},
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
	// Parent is intentionally omitted — the attachment resource owns it.
	// UpdateProgram's M-Z merge preserves the column when input.Parent == 0.
	return &zentaoapi.Program{
		Name:       m.Name.ValueString(),
		Begin:      m.Begin.ValueString(),
		End:        m.End.ValueString(),
		PM:         m.PM.ValueString(),
		Desc:       m.Desc.ValueString(),
		ACL:        m.ACL.ValueString(),
		Budget:     m.Budget.ValueString(),
		BudgetUnit: m.BudgetUnit.ValueString(),
		Whitelist:  m.Whitelist.ValueString(),
	}
}

func programWireBoolToTF(s string) types.Bool {
	return types.BoolValue(s == "1")
}

func programFromAPI(p *zentaoapi.Program) programResourceModel {
	return programResourceModel{
		ID:             types.StringValue(strconv.Itoa(p.ID)),
		Name:           types.StringValue(p.Name),
		Begin:          types.StringValue(p.Begin),
		End:            types.StringValue(p.End),
		Parent:         types.Int64Value(int64(p.Parent)),
		PM:             types.StringValue(p.PM),
		Desc:           types.StringValue(p.Desc),
		ACL:            types.StringValue(p.ACL),
		Budget:         types.StringValue(p.Budget),
		BudgetUnit:     types.StringValue(p.BudgetUnit),
		Whitelist:      types.StringValue(p.Whitelist),
		Code:           types.StringValue(p.Code),
		Status:         types.StringValue(p.Status),
		Type:           types.StringValue(p.Type),
		Category:       types.StringValue(p.Category),
		Lifetime:       types.StringValue(p.Lifetime),
		Vision:         types.StringValue(p.Vision),
		Attribute:      types.StringValue(p.Attribute),
		Model:          types.StringValue(p.Model),
		ProgramPath:    types.StringValue(p.Path),
		Grade:          types.Int64Value(int64(p.Grade)),
		Multiple:       programWireBoolToTF(p.Multiple),
		Parallel:       programWireBoolToTF(p.Parallel),
		HasProduct:     types.BoolValue(p.HasProduct == 1),
		WorkflowGroup:  types.Int64Value(int64(p.WorkflowGroup)),
		StoryType:      types.StringValue(p.StoryType),
		Days:           types.Int64Value(int64(p.Days)),
		FirstEnd:       types.StringValue(p.FirstEnd),
		OpenedBy:       types.StringValue(p.OpenedBy),
		OpenedDate:     types.StringValue(p.OpenedDate),
		LastEditedBy:   types.StringValue(p.LastEditedBy),
		LastEditedDate: types.StringValue(p.LastEditedDate),
		RealBegan:      types.StringValue(p.RealBegan),
		RealEnd:        types.StringValue(p.RealEnd),
		ClosedBy:       types.StringValue(p.ClosedBy),
		ClosedDate:     types.StringValue(p.ClosedDate),
		ClosedReason:   types.StringValue(p.ClosedReason),
		CanceledBy:     types.StringValue(p.CanceledBy),
		CanceledDate:   types.StringValue(p.CanceledDate),
		SuspendedDate:  types.StringValue(p.SuspendedDate),
		PO:             types.StringValue(p.PO),
		QD:             types.StringValue(p.QD),
		RD:             types.StringValue(p.RD),
		Team:           types.StringValue(p.Team),
		Progress:       types.StringValue(p.Progress),
		Percent:        types.StringValue(p.Percent),
		Estimate:       types.StringValue(p.Estimate),
		Consumed:       types.StringValue(p.Consumed),
		Left:           types.StringValue(p.Left),
		TeamCount:      types.Int64Value(int64(p.TeamCount)),
	}
}
