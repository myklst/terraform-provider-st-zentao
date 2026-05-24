package zentao

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
	_ resource.Resource                   = (*systemChildAttachmentResource)(nil)
	_ resource.ResourceWithConfigure      = (*systemChildAttachmentResource)(nil)
	_ resource.ResourceWithImportState    = (*systemChildAttachmentResource)(nil)
	_ resource.ResourceWithValidateConfig = (*systemChildAttachmentResource)(nil)
)

var systemIDRe = regexp.MustCompile(`^\d+$`)

type systemChildAttachmentResource struct {
	client *zentaoapi.Client
}

type systemChildAttachmentResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Parent types.String `tfsdk:"parent"`
	Child  types.String `tfsdk:"child"`
}

func NewSystemChildAttachmentResource() resource.Resource {
	return &systemChildAttachmentResource{}
}

func (r *systemChildAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_child_attachment"
}

func (r *systemChildAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Attaches a child application under a parent application. A child may belong to several parents; attaching is additive. Modelled as a separate resource because applications reference each other by id within one `for_each` block.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Composite edge id, `{parent}-{child}`.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"parent": schema.StringAttribute{
				Description:   "Parent (aggregating) application id.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.RegexMatches(systemIDRe, `must be a positive integer string`)},
				PlanModifiers: requiresReplace,
			},
			"child": schema.StringAttribute{
				Description:   "Child (member) application id.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.RegexMatches(systemIDRe, `must be a positive integer string`)},
				PlanModifiers: requiresReplace,
			},
		},
	}
}

func (r *systemChildAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *systemChildAttachmentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg systemChildAttachmentResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Parent.IsNull() || cfg.Parent.IsUnknown() || cfg.Child.IsNull() || cfg.Child.IsUnknown() {
		return
	}
	if cfg.Parent.ValueString() == cfg.Child.ValueString() {
		resp.Diagnostics.AddAttributeError(
			path.Root("child"),
			"Self-attach not allowed",
			fmt.Sprintf("`parent` and `child` must differ (both = %q).", cfg.Parent.ValueString()),
		)
	}
	for _, f := range []struct {
		name string
		v    types.String
	}{{"parent", cfg.Parent}, {"child", cfg.Child}} {
		if f.v.ValueString() == "0" {
			resp.Diagnostics.AddAttributeError(
				path.Root(f.name),
				fmt.Sprintf("`%s = \"0\"` is not a valid attachment", f.name),
				"Application ids are positive integers — remove this block instead.",
			)
		}
	}
}

func (r *systemChildAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemChildAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	parent, child, ok := systemAttachPair(plan.Parent.ValueString(), plan.Child.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	if _, err := r.client.AttachSystemChild(ctx, parent, child); err != nil {
		resp.Diagnostics.AddError("Attach application child failed", err.Error())
		return
	}
	plan.ID = types.StringValue(systemEdgeID(parent, child))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *systemChildAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior systemChildAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	parent, child, ok := systemAttachPair(prior.Parent.ValueString(), prior.Child.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	fetched, err := r.client.GetSystem(ctx, parent)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read parent application failed", err.Error())
		return
	}
	if !childInList(deref(fetched.Children), child) {
		// out-of-band detach — drop the attachment from state
		resp.State.RemoveResource(ctx)
		return
	}
	out := systemChildAttachmentResourceModel{
		ID:     types.StringValue(systemEdgeID(parent, child)),
		Parent: types.StringValue(strconv.FormatInt(parent, 10)),
		Child:  types.StringValue(strconv.FormatInt(child, 10)),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, out)...)
}

// Update is unreachable in practice — both parent and child are
// RequiresReplace, so any change destroys+recreates. It exists to satisfy
// the resource interface and re-asserts the computed id defensively.
func (r *systemChildAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemChildAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	parent, child, ok := systemAttachPair(plan.Parent.ValueString(), plan.Child.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	plan.ID = types.StringValue(systemEdgeID(parent, child))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *systemChildAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior systemChildAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	parent, child, ok := systemAttachPair(prior.Parent.ValueString(), prior.Child.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	if _, err := r.client.DetachSystemChild(ctx, parent, child); err != nil {
		if errors.Is(err, zentaoapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Detach application child failed", err.Error())
		return
	}
}

func (r *systemChildAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parentStr, childStr, found := strings.Cut(req.ID, "-")
	if !found || !systemIDRe.MatchString(parentStr) || !systemIDRe.MatchString(childStr) {
		resp.Diagnostics.AddError("Invalid import id", `import id must be "{parent}-{child}" (two positive integers)`)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parent"), parentStr)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("child"), childStr)...)
}

func systemEdgeID(parent, child int64) string {
	return strconv.FormatInt(parent, 10) + "-" + strconv.FormatInt(child, 10)
}

// childInList reports whether childID appears in a comma-separated id list.
func childInList(csv string, childID int64) bool {
	target := strconv.FormatInt(childID, 10)
	for _, p := range strings.Split(csv, ",") {
		if strings.TrimSpace(p) == target {
			return true
		}
	}
	return false
}

// systemAttachPair parses both ids and reports diagnostics on failure.
func systemAttachPair(parentStr, childStr string, diags interface {
	AddError(string, string)
}) (int64, int64, bool) {
	parent, err := strconv.ParseInt(parentStr, 10, 64)
	if err != nil || parent <= 0 {
		diags.AddError("Invalid parent id", parentStr)
		return 0, 0, false
	}
	child, err := strconv.ParseInt(childStr, 10, 64)
	if err != nil || child <= 0 {
		diags.AddError("Invalid child id", childStr)
		return 0, 0, false
	}
	return parent, child, true
}
