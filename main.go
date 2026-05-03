package main

import (
	"context"
	"log"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/myklst/terraform-provider-st-zentao/zentao"
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name st-zentao

func main() {
	addr := os.Getenv("PROVIDER_LOCAL_PATH")
	if addr == "" {
		addr = "registry.terraform.io/myklst/st-zentao"
	}

	if err := providerserver.Serve(context.Background(), zentao.New, providerserver.ServeOpts{
		Address: addr,
	}); err != nil {
		log.Fatal(err)
	}
}
