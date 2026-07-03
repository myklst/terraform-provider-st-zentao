package zentao

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*groupPrivsResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupPrivsResource)(nil)
	_ resource.ResourceWithImportState = (*groupPrivsResource)(nil)
)

var privRe = regexp.MustCompile(`^\w+-\w+$`)

type groupPrivsResource struct {
	client *zentaoapi.Client
}

type groupPrivsResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Group types.Int64  `tfsdk:"group"`
	Privs types.Set    `tfsdk:"privs"`
}

func NewGroupPrivsResource() resource.Resource { return &groupPrivsResource{} }

func (r *groupPrivsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_privs"
}

func (r *groupPrivsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the complete privilege set of a ZenTao permission group. " +
			"This resource owns the group's entire set: applying replaces it, destroying clears it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric group ID (same as `group`).",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group": schema.Int64Attribute{
				Description: "Numeric group ID whose privileges this resource manages. Changing it forces resource replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"privs": schema.SetAttribute{
				Description: "Privilege grants, each as `module-method` (e.g. `story-create`). An empty set asserts the group has no privileges.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(privRe, `each priv must be "module-method"`),
					),
				},
			},
		},
	}
}

func (r *groupPrivsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *groupPrivsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupPrivsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := plan.Group.ValueInt64()
	privs, diags := setToStrings(ctx, plan.Privs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetGroupPrivs(ctx, groupID, privs); err != nil {
		resp.Diagnostics.AddError("Set group privs failed", err.Error())
		return
	}
	r.readInto(ctx, groupID, &resp.State, &resp.Diagnostics)
}

func (r *groupPrivsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior groupPrivsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := prior.Group.ValueInt64()
	current, err := r.client.GetGroupPrivs(ctx, groupID)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read group privs failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, modelFromPrivs(ctx, groupID, current, &resp.Diagnostics))...)
}

func (r *groupPrivsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupPrivsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := plan.Group.ValueInt64()
	privs, diags := setToStrings(ctx, plan.Privs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetGroupPrivs(ctx, groupID, privs); err != nil {
		resp.Diagnostics.AddError("Set group privs failed", err.Error())
		return
	}
	r.readInto(ctx, groupID, &resp.State, &resp.Diagnostics)
}

func (r *groupPrivsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior groupPrivsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetGroupPrivs(ctx, prior.Group.ValueInt64(), nil); err != nil {
		if errors.Is(err, zentaoapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Clear group privs failed", err.Error())
	}
}

func (r *groupPrivsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import id", "import id must be a positive integer (the group id)")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group"), id)...)
}

// readInto re-fetches the server-side set after a write and writes it to state.
// The server stores exactly what is posted (no implicit dependent privs on this
// version), so the re-read set equals the planned set; using the server value
// keeps state authoritative against out-of-band edits.
func (r *groupPrivsResource) readInto(ctx context.Context, groupID int64, state *tfsdk.State, diags *diag.Diagnostics) {
	current, err := r.client.GetGroupPrivs(ctx, groupID)
	if err != nil {
		diags.AddError("Re-fetch group privs failed", err.Error())
		return
	}
	diags.Append(state.Set(ctx, modelFromPrivs(ctx, groupID, current, diags))...)
}

func modelFromPrivs(ctx context.Context, groupID int64, privs []string, diags *diag.Diagnostics) groupPrivsResourceModel {
	set, d := setFromStrings(ctx, privs)
	diags.Append(d...)
	return groupPrivsResourceModel{
		ID:    types.StringValue(strconv.FormatInt(groupID, 10)),
		Group: types.Int64Value(groupID),
		Privs: set,
	}
}

// setToStrings flattens a Terraform string set to a slice. A null/unknown set
// yields nil (the empty-clear case).
func setToStrings(ctx context.Context, v types.Set) ([]string, diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := v.ElementsAs(ctx, &out, false)
	return out, diags
}

// setFromStrings builds a Terraform string set; nil is normalised to an empty
// (non-null) set so an all-cleared group reads back as `privs = []`.
func setFromStrings(ctx context.Context, src []string) (types.Set, diag.Diagnostics) {
	if src == nil {
		src = []string{}
	}
	return types.SetValueFrom(ctx, types.StringType, src)
}
