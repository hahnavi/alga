GO ?= $(shell command -v go 2> /dev/null)
GO_TEST_FLAGS ?= -race
GO_BUILD_FLAGS ?=
GO_BUILD_GCFLAGS ?=

export GO111MODULE=on
export CGO_ENABLED=0
export GOWORK=off

PLUGIN_ID := com.alga.mattermost-plugin
PLUGIN_VERSION := 0.0.1
BUNDLE_NAME := $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz

DIST_DIR := dist
SERVER_DIST_DIR := server/dist

.DEFAULT_GOAL := all
.PHONY: all build bundle dist clean test lint vet fmt tidy deploy

all: vet test build bundle

build: $(SERVER_DIST_DIR)
	$(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o $(SERVER_DIST_DIR)/plugin .

bundle: build
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)/$(PLUGIN_ID)
	@cp plugin.json $(DIST_DIR)/$(PLUGIN_ID)/
	@mkdir -p $(DIST_DIR)/$(PLUGIN_ID)/server
	@cp -r $(SERVER_DIST_DIR) $(DIST_DIR)/$(PLUGIN_ID)/server/
	@cd $(DIST_DIR) && tar -cvzf $(BUNDLE_NAME) $(PLUGIN_ID)
	@echo "Plugin bundle: $(DIST_DIR)/$(BUNDLE_NAME)"

dist: bundle

vet:
	$(GO) vet ./...

test: vet
	$(GO) test $(GO_TEST_FLAGS) -v ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

deploy: bundle
	@echo "Deploy $(DIST_DIR)/$(BUNDLE_NAME) via System Console > Plugins > Plugin Management"
	@echo "Or copy to the Mattermost plugins directory and restart the server."

clean:
	@rm -rf $(DIST_DIR)
	@rm -rf $(SERVER_DIST_DIR)

$(SERVER_DIST_DIR):
	@mkdir -p $@

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all      Run vet, test, build, and bundle (default)"
	@echo "  build    Build plugin binary"
	@echo "  bundle   Create distributable tar.gz"
	@echo "  vet      Run go vet"
	@echo "  test     Run tests with race detection"
	@echo "  lint     Run golangci-lint"
	@echo "  fmt      Format Go code"
	@echo "  tidy     Run go mod tidy"
	@echo "  deploy   Build and show deployment instructions"
	@echo "  clean    Remove build artifacts"
