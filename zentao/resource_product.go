package zentao

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func NewProductResource() resource.Resource { return &productResource{} }

type productResource struct{}

func (r *productResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *productResource) Schema(_ context.Context, _ resource.SchemaRequest, _ *resource.SchemaResponse) {
}

func (r *productResource) Create(_ context.Context, _ resource.CreateRequest, _ *resource.CreateResponse) {
}
func (r *productResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse)    {}
func (r *productResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *productResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
