.PHONY: help build push test clean version-tag check-docker-username

# Configuration - can be overridden via environment variables
# Try multiple methods to detect Docker Hub username:
# 1. Check DOCKER_USERNAME env var
# 2. Try to extract from docker config.json
# 3. Try docker info (shows logged in user on some systems)
DOCKER_USERNAME ?= $(shell \
	if [ -n "$$DOCKER_USERNAME" ]; then \
		echo "$$DOCKER_USERNAME"; \
	elif [ -f ~/.docker/config.json ]; then \
		cat ~/.docker/config.json | grep -o '"username"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"username"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' | grep -v '^$$'; \
	fi \
)
DOCKER_REPO ?= dynipupdate
IMAGE_NAME ?= $(DOCKER_USERNAME)/$(DOCKER_REPO)
PLATFORMS ?= linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/riscv64

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
	@echo "Configuration (override via environment variables):"
	@echo "  DOCKER_USERNAME  - Docker Hub username (current: $(DOCKER_USERNAME))"
	@echo "  DOCKER_REPO      - Repository name (current: $(DOCKER_REPO))"
	@echo "  IMAGE_NAME       - Full image name (current: $(IMAGE_NAME))"
	@echo "  PLATFORMS        - Build platforms (current: $(PLATFORMS))"
	@echo "  DATE             - Build date (current: $(DATE))"
	@echo ""
	@echo "Quick Start:"
	@echo "  export DOCKER_USERNAME=your-username"
	@echo "  make build"
	@echo ""
	@echo "Examples:"
	@echo "  DOCKER_USERNAME=myuser make build            # Set username"
	@echo "  IMAGE_NAME=myuser/myrepo make build          # Override full image name"
	@echo "  PLATFORMS=linux/amd64,linux/arm64 make build # Custom platforms"

# Get the next version number for today
.PHONY: get-next-version
get-next-version:
	@$(eval NEXT_VERSION := $(shell ./scripts/get-next-version.sh $(IMAGE_NAME) $(DATE)))
	@echo $(NEXT_VERSION)

check-docker-username:
	@if echo "$(IMAGE_NAME)" | grep -q "^/"; then \
		echo "Error: DOCKER_USERNAME not set and could not be auto-detected."; \
		echo ""; \
		echo "Please either:"; \
		echo "  1. Set DOCKER_USERNAME environment variable:"; \
		echo "     export DOCKER_USERNAME=your-username"; \
		echo "     make build"; \
		echo ""; \
		echo "  2. Override IMAGE_NAME directly:"; \
		echo "     IMAGE_NAME=your-username/dynipupdate make build"; \
		echo ""; \
		echo "  3. Ensure you're logged in to Docker Hub:"; \
		echo "     docker login"; \
		exit 1; \
	fi

version-tag: check-docker-username
	@echo "Next version tag will be: $(DATE)$(shell ./scripts/get-next-version.sh $(IMAGE_NAME) $(DATE))"

test:
	@echo "Running Go tests..."
	go test -v ./...

build: check-docker-username test
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
