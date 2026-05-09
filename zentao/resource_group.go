package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*groupResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupResource)(nil)
	_ resource.ResourceWithImportState = (*groupResource)(nil)
)

type groupResource struct {
	client *zentaoapi.Client
}

// groupResourceModel mirrors the v1 surface area documented in the
// probe spec (docs/superpowers/specs/probe-group-controller.md §3.11).
// We deliberately do NOT surface vision/developer/acl/users/actions here;
// the wire layer keeps vision/developer for round-trip safety, but they
// are not user-managed in v1.
type groupResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Project types.Int64  `tfsdk:"project"`
	Name    types.String `tfsdk:"name"`
	Role    types.String `tfsdk:"role"`
	Desc    types.String `tfsdk:"desc"`
}

func NewGroupResource() resource.Resource { return &groupResource{} }

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao permission group. `project = 0` (default) is a system " +
			"(org-wide) group; `project > 0` is a project-scoped group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao group ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"project": schema.Int64Attribute{
				Description: "Parent project id. `0` = system (org-wide) group; positive integer = project-scoped group. Changing this forces resource replacement.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Group display name. Must be unique within its scope.",
				Required:    true,
			},
			"role": schema.StringAttribute{
				Description:   "Free-text role label.",
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
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateGroup(ctx, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Create group failed", err.Error())
		return
	}
	fetched, err := r.client.GetGroup(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Re-fetch after create failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupFromAPI(fetched))...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetGroup(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupFromAPI(fetched))...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior groupResourceModel
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
	updated, err := r.client.UpdateGroup(ctx, apiInput)
	// Probe spec §0.2: POST /group-edit-<id>.json on a non-existent id
	// silently returns success. UpdateGroup re-reads after the POST
	// and surfaces ErrNotFound when the row vanished. We surface this as a
	// loud error rather than silently removing state mid-Update — losing
	// state inside an Update step is unusual enough to warrant explicit
	// operator attention.
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Update group failed: row no longer exists",
			fmt.Sprintf("ZenTao silently accepted POST /group-edit-%d.json but the row "+
				"was not present on the server-side re-read. The group may have been "+
				"deleted out-of-band. Run `terraform apply` again to recreate it, or "+
				"investigate before retrying.", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Update group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupFromAPI(updated))...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	// DeleteGroup is a destructive GET (probe spec §0.1, §2.5) and
	// is idempotent on missing rows. The wrapper handles all the safety
	// nuances; the resource simply propagates errors.
	if err := r.client.DeleteGroup(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete group failed", err.Error())
		return
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toAPI projects a Terraform plan into the zentaoapi.Group wire shape.
// vision/developer are intentionally left zero — the wire layer fills
// vision="rnd" by default and the server ignores developer on Update.
func (m *groupResourceModel) toAPI() *zentaoapi.Group {
	return &zentaoapi.Group{
		Project: int(m.Project.ValueInt64()),
		Name:    m.Name.ValueString(),
		Role:    m.Role.ValueString(),
		Desc:    m.Desc.ValueString(),
	}
}

func groupFromAPI(g *zentaoapi.Group) groupResourceModel {
	return groupResourceModel{
		ID:      types.StringValue(strconv.Itoa(g.ID)),
		Project: types.Int64Value(int64(g.Project)),
		Name:    types.StringValue(g.Name),
		Role:    types.StringValue(g.Role),
		Desc:    types.StringValue(g.Desc),
	}
}
