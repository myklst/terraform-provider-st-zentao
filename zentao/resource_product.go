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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*productResource)(nil)
	_ resource.ResourceWithConfigure   = (*productResource)(nil)
	_ resource.ResourceWithImportState = (*productResource)(nil)
)

// Allowed enums per ZenTao v2 API documentation.
var (
	productTypeEnum = []string{"normal", "branch", "platform"}
	productACLEnum  = []string{"open", "private"}
)

type productResource struct {
	client *zentaoapi.Client
}

type productResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Program   types.Int64  `tfsdk:"program"`
	Name      types.String `tfsdk:"name"`
	Code      types.String `tfsdk:"code"`
	Shadow    types.Bool   `tfsdk:"shadow"`
	Line      types.Int64  `tfsdk:"line"`
	Type      types.String `tfsdk:"type"`
	Status    types.String `tfsdk:"status"`
	Desc      types.String `tfsdk:"desc"`
	PO        types.String `tfsdk:"po"`
	QD        types.String `tfsdk:"qd"`
	RD        types.String `tfsdk:"rd"`
	Reviewers types.List   `tfsdk:"reviewers"`
	ACL       types.String `tfsdk:"acl"`
	Whitelist types.List   `tfsdk:"whitelist"`
	Deleted   types.Bool   `tfsdk:"deleted"`
}

func NewProductResource() resource.Resource { return &productResource{} }

func (r *productResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *productResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useStateForInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	useStateForBool := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	useStateForList := []planmodifier.List{listplanmodifier.UseStateForUnknown()}
	useStateForString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao product.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Numeric ZenTao product ID.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"program": schema.Int64Attribute{
				Description:   "Associated program id.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"name": schema.StringAttribute{
				Description: "Product display name.",
				Required:    true,
			},
			"code": schema.StringAttribute{
				Description:   "Product short code.",
				Computed:      true,
				PlanModifiers: useStateForString,
			},
			"shadow": schema.BoolAttribute{
				Description:   "Whether a shadow product.",
				Computed:      true,
				PlanModifiers: useStateForBool,
			},
			"line": schema.Int64Attribute{
				Description:   "Associated product line id.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForInt,
			},
			"type": schema.StringAttribute{
				Description:   "Product type. One of: " + commaJoin(productTypeEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(productTypeEnum...)},
			},
			"status": schema.StringAttribute{
				Description: "Product status.",
				Computed:    true,
			},
			"desc": schema.StringAttribute{
				Description:   "Description.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				PlanModifiers: useStateForString,
			},
			"po": schema.StringAttribute{
				Description:   "Product Owner username. Defaults to the requesting account if omitted.",
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
			"reviewers": schema.ListAttribute{
				Description:   "Reviewer usernames.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: useStateForList,
			},
			"acl": schema.StringAttribute{
				Description:   "Access control. One of: " + commaJoin(productACLEnum) + ".",
				Optional:      true,
				Computed:      true,
				PlanModifiers: useStateForString,
				Validators:    []validator.String{stringvalidator.OneOf(productACLEnum...)},
			},
			"whitelist": schema.ListAttribute{
				Description:   "Whitelisted usernames granted access to this product.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: useStateForList,
			},
			"deleted": schema.BoolAttribute{
				Description:   "Whether deleted.",
				Computed:      true,
				PlanModifiers: useStateForBool,
			},
		},
	}
}

func (r *productResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *productResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan productResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiInput, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateProduct(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Create product failed", err.Error())
		return
	}
	if created.ID == nil {
		resp.Diagnostics.AddError("Create product failed", "server returned no id")
		return
	}
	state, diags := productFromAPI(ctx, created)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *productResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior productResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	fetched, err := r.client.GetProduct(ctx, id)
	if errors.Is(err, zentaoapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read product failed", err.Error())
		return
	}
	state, diags := productFromAPI(ctx, fetched)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *productResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior productResourceModel
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
	updated, err := r.client.UpdateProduct(ctx, apiInput)
	if err != nil {
		resp.Diagnostics.AddError("Update product failed", err.Error())
		return
	}
	state, diags := productFromAPI(ctx, updated)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *productResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var prior productResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(prior.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid id in state", err.Error())
		return
	}
	if err := r.client.DeleteProduct(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete product failed", err.Error())
		return
	}
}

func (r *productResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (m *productResourceModel) toAPI(ctx context.Context) (*zentaoapi.Product, diag.Diagnostics) {
	reviewers, diags := optStrList(ctx, m.Reviewers)
	whitelist, d := optStrList(ctx, m.Whitelist)
	diags.Append(d...)
	return &zentaoapi.Product{
		Name:      optString(m.Name),
		Program:   optInt64(m.Program),
		Line:      optInt64(m.Line),
		Type:      optString(m.Type),
		Desc:      optString(m.Desc),
		ACL:       optString(m.ACL),
		PO:        optString(m.PO),
		QD:        optString(m.QD),
		RD:        optString(m.RD),
		Reviewers: reviewers,
		Whitelist: whitelist,
	}, diags
}

func productFromAPI(ctx context.Context, p *zentaoapi.Product) (productResourceModel, diag.Diagnostics) {
	reviewers, diags := stringListFromSlice(ctx, deref(p.Reviewers))
	whitelist, d := stringListFromSlice(ctx, deref(p.Whitelist))
	diags.Append(d...)
	return productResourceModel{
		ID:        types.StringValue(strconv.FormatInt(deref(p.ID), 10)),
		Code:      types.StringValue(deref(p.Code)),
		Shadow:    types.BoolValue(deref(p.Shadow)),
		Name:      types.StringValue(deref(p.Name)),
		Program:   types.Int64Value(deref(p.Program)),
		Line:      types.Int64Value(deref(p.Line)),
		Type:      types.StringValue(deref(p.Type)),
		Desc:      types.StringValue(deref(p.Desc)),
		ACL:       types.StringValue(deref(p.ACL)),
		PO:        types.StringValue(deref(p.PO)),
		QD:        types.StringValue(deref(p.QD)),
		RD:        types.StringValue(deref(p.RD)),
		Reviewers: reviewers,
		Whitelist: whitelist,
		Status:    types.StringValue(deref(p.Status)),
		Deleted:   types.BoolValue(deref(p.Deleted)),
	}, diags
}
