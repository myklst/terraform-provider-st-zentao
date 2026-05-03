CUSTOM_PROVIDER_NAME ?= terraform-provider-st-zentao
CUSTOM_PROVIDER_URL  ?= example.local/myklst/st-zentao
VERSION              ?= 0.1.0

UNAME := $(shell uname)

.PHONY: install-local-custom-provider
install-local-custom-provider: darwin_arm64 linux_amd64

darwin_arm64:
ifneq ($(UNAME), Darwin)
	$(info skip darwin_arm64)
else
	export PROVIDER_LOCAL_PATH='$(CUSTOM_PROVIDER_URL)'
	GOOS=darwin GOARCH=arm64 go install .
	GO_BIN="$$(go env GOPATH)/bin"; \
	HOME_DIR="$$(ls -d ~)"; \
	mkdir -p $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/$(VERSION)/darwin_arm64/; \
	cp $$GO_BIN/$(CUSTOM_PROVIDER_NAME) $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/$(VERSION)/darwin_arm64/$(CUSTOM_PROVIDER_NAME)
endif

linux_amd64:
ifneq ($(UNAME), Linux)
	$(info skip linux_amd64)
else
	export PROVIDER_LOCAL_PATH='$(CUSTOM_PROVIDER_URL)'
	GOOS=linux GOARCH=amd64 go install .
	GO_BIN="$$(go env GOPATH)/bin"; \
	HOME_DIR="$$(ls -d ~)"; \
	mkdir -p $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/$(VERSION)/linux_amd64/; \
	cp $$GO_BIN/$(CUSTOM_PROVIDER_NAME) $$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/$(VERSION)/linux_amd64/$(CUSTOM_PROVIDER_NAME)
endif

.PHONY: generate-docs
generate-docs:
	go generate ./...

.PHONY: go-test-unit
go-test-unit:
	go test -v -cover ./zentao/... ./zentaoAPI/...

.PHONY: go-test-acc
go-test-acc:
	TF_ACC=1 go test -v -timeout 30m ./zentao/...

.PHONY: go-lint
go-lint:
	golangci-lint run ./...
