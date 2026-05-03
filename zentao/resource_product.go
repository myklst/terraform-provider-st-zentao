package zentao

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

var (
	_ resource.Resource                = (*productResource)(nil)
	_ resource.ResourceWithConfigure   = (*productResource)(nil)
	_ resource.ResourceWithImportState = (*productResource)(nil)
)

type productResource struct {
	client *zentaoapi.Client
}

type productResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Code        types.String `tfsdk:"code"`
	Status      types.String `tfsdk:"status"`
	Description types.String `tfsdk:"description"`
	ACL         types.String `tfsdk:"acl"`
	Type        types.String `tfsdk:"type"`
}

func NewProductResource() resource.Resource { return &productResource{} }

func (r *productResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *productResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ZenTao product. The code field cannot be changed in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ZenTao product ID (stringified).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Product display name.",
				Required:    true,
			},
			"code": schema.StringAttribute{
				Description: "Product short code (slug). ZenTao does not allow editing this in place.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Product status. Defaults to \"normal\".",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("normal"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Optional description (mapped to ZenTao 'desc').",
				Optional:    true,
			},
			"acl": schema.StringAttribute{
				Description: "Access control. Defaults to \"private\".",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("private"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Description: "Product type. Defaults to \"normal\".",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("normal"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

// Create / Read / Update / Delete / ImportState are implemented in the next tasks.
func (r *productResource) Create(_ context.Context, _ resource.CreateRequest, _ *resource.CreateResponse) {
}
func (r *productResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse)       {}
func (r *productResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *productResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *productResource) ImportState(_ context.Context, _ resource.ImportStateRequest, _ *resource.ImportStateResponse) {
}

// Used by Create/Read/Update; defined here for forward references in following tasks.
func (m *productResourceModel) toAPI() *zentaoapi.Product {
	return &zentaoapi.Product{
		Name:        m.Name.ValueString(),
		Code:        m.Code.ValueString(),
		Status:      m.Status.ValueString(),
		Description: m.Description.ValueString(),
		ACL:         m.ACL.ValueString(),
		Type:        m.Type.ValueString(),
	}
}

func fromAPI(p *zentaoapi.Product) productResourceModel {
	return productResourceModel{
		ID:          types.StringValue(strconv.Itoa(p.ID)),
		Name:        types.StringValue(p.Name),
		Code:        types.StringValue(p.Code),
		Status:      types.StringValue(p.Status),
		Description: types.StringValue(p.Description),
		ACL:         types.StringValue(p.ACL),
		Type:        types.StringValue(p.Type),
	}
}

// silence unused-import warnings until Tasks 15–17 use them
var _ = errors.Is
var _ = path.Root
