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
# For local builds, use a simple name; for push, require username
IMAGE_NAME ?= $(if $(DOCKER_USERNAME),$(DOCKER_USERNAME)/$(DOCKER_REPO),$(DOCKER_REPO))
PLATFORMS ?= linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/riscv64

help:
	@echo "Dynamic DNS Updater - Build Targets"
	@echo ""
	@echo "  make build       - Build Docker images (default: all platforms, no push)"
	@echo "  make push        - Push previously built images to Docker Hub"
	@echo "  make build-push  - Build and push in one step (default: all platforms)"
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
	@echo "Quick Start (Local Development):"
	@echo "  make build                    # Build locally without pushing"
	@echo ""
	@echo "Quick Start (Push to Registry):"
	@echo "  export DOCKER_USERNAME=your-username"
	@echo "  make build-push"
	@echo ""
	@echo "Examples:"
	@echo "  make build                                        # Build all platforms (no push)"
	@echo "  PLATFORMS=linux/amd64 make build                  # Build just amd64 (for testing)"
	@echo "  PLATFORMS=linux/arm64 make build                  # Build just arm64 (for testing)"
	@echo "  make push                                         # Push what you built"
	@echo "  make build-push                                   # Build all platforms and push"
	@echo "  PLATFORMS=linux/amd64,linux/arm64 make build-push # Build specific platforms and push"

check-docker-username:
	@if [ -z "$(DOCKER_USERNAME)" ]; then \
		echo "Error: DOCKER_USERNAME not set and could not be auto-detected."; \
		echo ""; \
		echo "Pushing to Docker Hub requires a username."; \
		echo ""; \
		echo "Please either:"; \
		echo "  1. Set DOCKER_USERNAME environment variable:"; \
		echo "     export DOCKER_USERNAME=your-username"; \
		echo "     make build-push"; \
		echo ""; \
		echo "  2. Override IMAGE_NAME directly:"; \
		echo "     IMAGE_NAME=your-username/dynipupdate make build-push"; \
		echo ""; \
		echo "  3. Ensure you're logged in to Docker Hub:"; \
		echo "     docker login"; \
		echo ""; \
		echo "For local builds only, use: make build"; \
		exit 1; \
	fi

version-tag: check-docker-username
	@echo "Next version tag will be: $(shell git log -1 --date=format:'%Y%m%d-%H%M%S' --format=%cd)"

test:
	@echo "Running Go unit tests..."
	go test -v ./...

# Build Docker images (without pushing)
# Supports building specific platforms via PLATFORMS variable
build: check-docker-username test
	@echo "Building Docker images..."
	@$(eval VERSION_TAG := $(shell git log -1 --date=format:'%Y%m%d-%H%M%S' --format=%cd))
	@echo "Building version: $(VERSION_TAG)"
	@echo "Platforms: $(PLATFORMS)"
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(IMAGE_NAME):latest \
		-t $(IMAGE_NAME):$(VERSION_TAG) \
		.
	@echo ""
	@echo "✓ Successfully built (not pushed):"
	@echo "  - $(IMAGE_NAME):latest"
	@echo "  - $(IMAGE_NAME):$(VERSION_TAG)"
	@echo ""
	@echo "Platforms: $(PLATFORMS)"
	@echo ""
	@echo "To push these images, run: make push"

# Push previously built images to Docker Hub
push: check-docker-username
	@echo "Pushing images to Docker Hub..."
	@if ! docker buildx imagetools inspect $(IMAGE_NAME):latest >/dev/null 2>&1 && \
	    ! docker image inspect $(IMAGE_NAME):latest >/dev/null 2>&1; then \
		echo "Error: Image $(IMAGE_NAME):latest not found."; \
		echo "Run 'make build' first to build the images."; \
		exit 1; \
	fi
	@echo "Pushing $(IMAGE_NAME):latest..."
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(IMAGE_NAME):latest \
		--push \
		.
	@# Find and push the most recent timestamp tag if it exists locally
	@$(eval LATEST_TAG := $(shell docker images $(IMAGE_NAME) --format "{{.Tag}}" 2>/dev/null | grep -E '^[0-9]{8}-[0-9]{6}$$' | sort -r | head -1))
	@if [ -n "$(LATEST_TAG)" ]; then \
		echo "Pushing $(IMAGE_NAME):$(LATEST_TAG)..."; \
		docker buildx build \
			--platform $(PLATFORMS) \
			-t $(IMAGE_NAME):$(LATEST_TAG) \
			--push \
			.; \
	fi
	@echo ""
	@echo "✓ Successfully pushed:"
	@echo "  - $(IMAGE_NAME):latest"
	@if [ -n "$(LATEST_TAG)" ]; then \
		echo "  - $(IMAGE_NAME):$(LATEST_TAG)"; \
	fi

# Build and push in one step
# Supports building specific platforms via PLATFORMS variable
build-push: check-docker-username test
	@echo "Building and pushing Docker images..."
	@$(eval VERSION_TAG := $(shell git log -1 --date=format:'%Y%m%d-%H%M%S' --format=%cd))
	@echo "Building version: $(VERSION_TAG)"
	@echo "Platforms: $(PLATFORMS)"
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
	@echo ""
	@echo "Platforms: $(PLATFORMS)"

clean:
	@echo "Cleaning build artifacts..."
	rm -f dynipupdate
	go clean
	@echo "✓ Clean complete"
