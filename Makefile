.PHONY: help build push test clean version-tag

# Configuration
IMAGE_NAME := richleigh/dynipupdate
PLATFORMS := linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/riscv64

# Get current date in YYYYMMDD format
DATE := $(shell date +%Y%m%d)

help:
	@echo "Dynamic DNS Updater - Build Targets"
	@echo ""
	@echo "  make build       - Build multi-platform Docker image and push with auto-incrementing tag"
	@echo "  make test        - Run Go unit tests"
	@echo "  make version-tag - Show what the next version tag will be"
	@echo "  make clean       - Clean build artifacts"
	@echo ""
	@echo "Current configuration:"
	@echo "  Image: $(IMAGE_NAME)"
	@echo "  Platforms: $(PLATFORMS)"
	@echo "  Date: $(DATE)"

# Get the next version number for today
.PHONY: get-next-version
get-next-version:
	@$(eval NEXT_VERSION := $(shell ./scripts/get-next-version.sh $(IMAGE_NAME) $(DATE)))
	@echo $(NEXT_VERSION)

version-tag:
	@echo "Next version tag will be: $(DATE)$(shell ./scripts/get-next-version.sh $(IMAGE_NAME) $(DATE))"

test:
	@echo "Running Go tests..."
	go test -v ./...

build: test
	@echo "Building and pushing multi-platform Docker image..."
	@$(eval VERSION_NUM := $(shell ./scripts/get-next-version.sh $(IMAGE_NAME) $(DATE)))
	@$(eval FULL_TAG := $(DATE)$(VERSION_NUM))
	@echo "Building version: $(FULL_TAG)"
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(IMAGE_NAME):latest \
		-t $(IMAGE_NAME):$(FULL_TAG) \
		--push \
		.
	@echo ""
	@echo "✓ Successfully built and pushed:"
	@echo "  - $(IMAGE_NAME):latest"
	@echo "  - $(IMAGE_NAME):$(FULL_TAG)"

clean:
	@echo "Cleaning build artifacts..."
	rm -f dynipupdate
	go clean
	@echo "✓ Clean complete"
