BINARY_DIR = ./bin
GO = go

.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help message
	@echo ""
	@echo "Usage:"
	@echo "  make \033[36m<target>\033[0m"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "  \033[1;36mAvailable targets:\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""
	@echo "Run 'make <target>' (e.g. make build, make run-server)"
	@echo ""

.PHONY: all build
all: build

build: ## Build all binaries
	@mkdir -p $(BINARY_DIR)
	@$(GO) build -o $(BINARY_DIR)/broadcast    ./cmd/broadcast
	@$(GO) build -o $(BINARY_DIR)/listen       ./cmd/listen
	@$(GO) build -o $(BINARY_DIR)/server       ./cmd/server
	@$(GO) build -o $(BINARY_DIR)/clearlink    ./cmd/clearlink
	@chmod +x $(BINARY_DIR)/broadcast $(BINARY_DIR)/listen $(BINARY_DIR)/server $(BINARY_DIR)/clearlink

build-broadcast: ## Build only the broadcast binary
	@mkdir -p $(BINARY_DIR)
	@$(GO) build -o $(BINARY_DIR)/broadcast    ./cmd/broadcast
	@chmod +x $(BINARY_DIR)/broadcast

build-listen: ## Build only the listen binary
	@mkdir -p $(BINARY_DIR)
	@$(GO) build -o $(BINARY_DIR)/listen       ./cmd/listen
	@chmod +x $(BINARY_DIR)/listen

build-server: ## Build only the server binary
	@mkdir -p $(BINARY_DIR)
	@$(GO) build -o $(BINARY_DIR)/server       ./cmd/server
	@chmod +x $(BINARY_DIR)/server

build-clearlink: ## Build only the clearlink binary
	@mkdir -p $(BINARY_DIR)
	@$(GO) build -o $(BINARY_DIR)/clearlink    ./cmd/clearlink
	@chmod +x $(BINARY_DIR)/clearlink

.PHONY: run-broadcast run-listen run-server run-clearlink
run-broadcast: build-broadcast ## Build and run the broadcast binary
	@$(BINARY_DIR)/broadcast $(ARGS)

run-listen: build-listen ## Build and run the listen binary
	@$(BINARY_DIR)/listen $(ARGS)

run-server: build-server ## Build and run the server binary
	@$(BINARY_DIR)/server $(ARGS)

run-clearlink: build-clearlink ## Build and run the clearlink binary
	@$(BINARY_DIR)/clearlink $(ARGS)
