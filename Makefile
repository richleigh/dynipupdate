.PHONY: help build push build-push test clean version-tag check-docker-username

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

help:
	@echo "Dynamic DNS Updater - Build Targets"
	@echo ""
	@echo "  make build       - Build for current platform only (no push)"
	@echo "  make push        - Push previously built images to Docker Hub"
	@echo "  make build-push  - Build and push in one step (convenience)"
	@echo "  make test        - Run Go unit tests"
	@echo "  make version-tag - Show what the next version tag will be"
	@echo "  make clean       - Clean build artifacts"
	@echo ""
	@echo "Configuration (override via environment variables):"
	@echo "  DOCKER_USERNAME  - Docker Hub username (current: $(DOCKER_USERNAME))"
	@echo "  DOCKER_REPO      - Repository name (current: $(DOCKER_REPO))"
	@echo "  IMAGE_NAME       - Full image name (current: $(IMAGE_NAME))"
	@echo "  PLATFORMS        - Build platforms (current: $(PLATFORMS))"
	@echo ""
	@echo "Quick Start:"
	@echo "  export DOCKER_USERNAME=your-username"
	@echo "  make build-push"
	@echo ""
	@echo "Examples:"
	@echo "  make build                                   # Build locally only"
	@echo "  make build-push                              # Build and push"
	@echo "  DOCKER_USERNAME=myuser make build-push      # Override username"
	@echo "  IMAGE_NAME=myuser/myrepo make build-push    # Override full image name"
	@echo "  PLATFORMS=linux/amd64,linux/arm64 make build # Build for specific platforms"

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
	@echo "Next version tag will be: $(shell date -u +%Y%m%d-%H%M%S)"

test:
	@echo "Running Go unit tests..."
	go test -v ./...

# Build images locally without pushing
# Note: Multi-platform builds cannot be loaded locally, so this builds for current platform only
build: check-docker-username test
	@echo "Building Docker image for current platform..."
	@$(eval VERSION_TAG := $(shell date -u +%Y%m%d-%H%M%S))
	@$(eval CURRENT_PLATFORM := $(shell docker version --format '{{.Server.Os}}/{{.Server.Arch}}'))
	@echo "Building version: $(VERSION_TAG) for $(CURRENT_PLATFORM)"
	docker buildx build \
		--platform $(CURRENT_PLATFORM) \
		-t $(IMAGE_NAME):latest \
		-t $(IMAGE_NAME):$(VERSION_TAG) \
		--load \
		.
	@echo ""
	@echo "✓ Successfully built locally:"
	@echo "  - $(IMAGE_NAME):latest"
	@echo "  - $(IMAGE_NAME):$(VERSION_TAG)"
	@echo ""
	@echo "Note: This built for $(CURRENT_PLATFORM) only."
	@echo "For multi-platform builds, use: make build-push"

# Push previously built images
push: check-docker-username
	@echo "Pushing images to Docker Hub..."
	@if ! docker image inspect $(IMAGE_NAME):latest >/dev/null 2>&1; then \
		echo "Error: Image $(IMAGE_NAME):latest not found locally."; \
		echo "Run 'make build' first to build the images."; \
		exit 1; \
	fi
	docker push $(IMAGE_NAME):latest
	@# Find and push the most recent timestamp tag
	@$(eval LATEST_TAG := $(shell docker images $(IMAGE_NAME) --format "{{.Tag}}" | grep -E '^[0-9]{8}-[0-9]{6}$$' | sort -r | head -1))
	@if [ -n "$(LATEST_TAG)" ]; then \
		docker push $(IMAGE_NAME):$(LATEST_TAG); \
		echo ""; \
		echo "✓ Successfully pushed:"; \
		echo "  - $(IMAGE_NAME):latest"; \
		echo "  - $(IMAGE_NAME):$(LATEST_TAG)"; \
	else \
		echo ""; \
		echo "✓ Successfully pushed:"; \
		echo "  - $(IMAGE_NAME):latest"; \
	fi

# Build and push in one step (convenience)
build-push: check-docker-username test
	@echo "Building and pushing multi-platform Docker images..."
	@$(eval VERSION_TAG := $(shell date -u +%Y%m%d-%H%M%S))
	@echo "Building version: $(VERSION_TAG)"
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(IMAGE_NAME):latest \
		-t $(IMAGE_NAME):$(VERSION_TAG) \
		--push \
		.
	@echo ""
	@echo "✓ Successfully built and pushed:"
	@echo "  - $(IMAGE_NAME):latest"
	@echo "  - $(IMAGE_NAME):$(VERSION_TAG)"

clean:
	@echo "Cleaning build artifacts..."
	rm -f dynipupdate
	go clean
	@echo "✓ Clean complete"
