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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                   = (*programParentAttachmentResource)(nil)
	_ resource.ResourceWithConfigure      = (*programParentAttachmentResource)(nil)
	_ resource.ResourceWithImportState    = (*programParentAttachmentResource)(nil)
	_ resource.ResourceWithValidateConfig = (*programParentAttachmentResource)(nil)
)

var programIDRe = regexp.MustCompile(`^\d+$`)

type programParentAttachmentResource struct {
	client *zentaoapi.Client
}

type programParentAttachmentResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Program types.String `tfsdk:"program"`
	Parent  types.String `tfsdk:"parent"`
}

func NewProgramParentAttachmentResource() resource.Resource {
	return &programParentAttachmentResource{}
}

func (r *programParentAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program_parent_attachment"
}

func (r *programParentAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "Attaches a child program under a parent program. Use this when you need to reference programs by id within the same `for_each` block — Terraform's self-reference cycle prevents writing `parent` directly on `st-zentao_program`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Child program id (same as `program`).",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"program": schema.StringAttribute{
				Description: "Child program id.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(programIDRe, `must be a positive integer string`)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"parent": schema.StringAttribute{
				Description: "Parent program id.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(programIDRe, `must be a positive integer string`)},
			},
		},
	}
}

func (r *programParentAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *programParentAttachmentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg programParentAttachmentResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Program.IsNull() || cfg.Program.IsUnknown() || cfg.Parent.IsNull() || cfg.Parent.IsUnknown() {
		return
	}
	if cfg.Program.ValueString() == cfg.Parent.ValueString() {
		resp.Diagnostics.AddAttributeError(
			path.Root("parent"),
			"Self-attach not allowed",
			fmt.Sprintf("`program` and `parent` must differ (both = %q).", cfg.Program.ValueString()),
		)
	}
	if cfg.Parent.ValueString() == "0" {
		resp.Diagnostics.AddAttributeError(
			path.Root("parent"),
			"`parent = \"0\"` is not a valid attachment",
			"Top-level programs need no attachment resource — remove this block instead.",
		)
	}
}

func (r *programParentAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan programParentAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	child, parent, ok := atoi64Pair(plan.Program.ValueString(), plan.Parent.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	// P3 collision: refuse to overwrite an existing attachment that points
	// somewhere else, but stay idempotent when the attachment already
	// matches the plan (recovers from interrupted applies).
	current, err := r.client.GetProgram(ctx, child)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError("Child program not found", fmt.Sprintf("program id %d does not exist", child))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read child program failed", err.Error())
		return
	}
	currentParent := derefInt64(current.Parent)
	switch currentParent {
	case 0:
		// first attachment — proceed
	case parent:
		// already attached as planned — idempotent
		plan.ID = plan.Program
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	default:
		resp.Diagnostics.AddError(
			"Child program already has a different parent",
			fmt.Sprintf("program %d is currently attached to %d, plan wants %d. Run `terraform import st-zentao_program_parent_attachment.<name> %d` to adopt the existing attachment, then re-plan.",
				child, currentParent, parent, child),
		)
		return
	}
	if _, err := r.client.SetProgramParent(ctx, child, parent); err != nil {
		resp.Diagnostics.AddError("Set program parent failed", err.Error())
		return
	}
	plan.ID = plan.Program
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *programParentAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior programParentAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	child, err := strconv.ParseInt(prior.Program.ValueString(), 10, 64)
	if err != nil || child <= 0 {
		resp.Diagnostics.AddError("Invalid program id in state", prior.Program.ValueString())
		return
	}
	fetched, err := r.client.GetProgram(ctx, child)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read child program failed", err.Error())
		return
	}
	fetchedParent := derefInt64(fetched.Parent)
	if fetchedParent == 0 {
		// out-of-band detach — drop the attachment from state
		resp.State.RemoveResource(ctx)
		return
	}
	out := programParentAttachmentResourceModel{
		ID:      types.StringValue(strconv.FormatInt(child, 10)),
		Program: types.StringValue(strconv.FormatInt(child, 10)),
		Parent:  types.StringValue(strconv.FormatInt(fetchedParent, 10)),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, out)...)
}

func (r *programParentAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan programParentAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	child, parent, ok := atoi64Pair(plan.Program.ValueString(), plan.Parent.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	if _, err := r.client.SetProgramParent(ctx, child, parent); err != nil {
		resp.Diagnostics.AddError("Set program parent failed", err.Error())
		return
	}
	plan.ID = plan.Program
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *programParentAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior programParentAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	child, err := strconv.ParseInt(prior.Program.ValueString(), 10, 64)
	if err != nil || child <= 0 {
		resp.Diagnostics.AddError("Invalid program id in state", prior.Program.ValueString())
		return
	}
	if _, err := r.client.SetProgramParent(ctx, child, 0); err != nil {
		if errors.Is(err, zentaoapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Detach program parent failed", err.Error())
		return
	}
}

func (r *programParentAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !programIDRe.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid import id", "import id must be a positive integer (the child program id)")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("program"), req.ID)...)
}

// atoi64Pair parses both ids and reports diagnostics on failure.
func atoi64Pair(programStr, parentStr string, diags interface {
	AddError(string, string)
}) (int64, int64, bool) {
	child, err := strconv.ParseInt(programStr, 10, 64)
	if err != nil || child <= 0 {
		diags.AddError("Invalid program id", programStr)
		return 0, 0, false
	}
	parent, err := strconv.ParseInt(parentStr, 10, 64)
	if err != nil || parent <= 0 {
		diags.AddError("Invalid parent id", parentStr)
		return 0, 0, false
	}
	return child, parent, true
}
