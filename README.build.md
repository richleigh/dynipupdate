# Build Instructions

This project uses a Makefile with timestamp-based version tags for Docker builds.

## Quick Start

1. **Set your Docker Hub username:**
   ```bash
   export DOCKER_USERNAME=your-username
   ```

   **Tip:** Add this to your `~/.bashrc`, `~/.zshrc`, or similar to make it permanent:
   ```bash
   echo 'export DOCKER_USERNAME=your-username' >> ~/.bashrc
   ```

2. **Build and push:**
   ```bash
   make build-push
   ```

   Or build locally first, then push separately:
   ```bash
   make build  # Build for current platform only
   make push   # Push to Docker Hub
   ```

## Configuration

The build system supports several environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DOCKER_USERNAME` | Your Docker Hub username | Attempts auto-detection (unreliable) - **recommended to set explicitly** |
| `DOCKER_REPO` | Repository name | `dynipupdate` |
| `IMAGE_NAME` | Full image name (overrides above) | `${DOCKER_USERNAME}/${DOCKER_REPO}` |
| `PLATFORMS` | Target platforms | `linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/riscv64` |

**Note:** Auto-detection of `DOCKER_USERNAME` may not work reliably across different Docker Desktop versions or when using credential helpers. It's recommended to set it explicitly via environment variable.

## Available Targets

```bash
make build       # Build locally for current platform (no push)
make push        # Push previously built images to Docker Hub
make build-push  # Build multi-platform and push (convenience)
make test        # Run Go unit tests only
make version-tag # Show what the next version tag will be
make clean       # Clean build artifacts
make help        # Show help message
```

## Version Tagging

The build system automatically creates version tags based on the build timestamp:

- **Format:** `YYYYMMDD-HHMMSS` (UTC time)
- **Example:** `20251109-143022`

### Why Timestamps?

Using timestamps is simple and eliminates race conditions:
- **Unique** - virtually impossible to have two builds in the same second
- **No coordination needed** - no git tags, no API calls, no state files
- **Sortable** - naturally sorts chronologically
- **Works everywhere** - multiple developers, CI/CD, build servers

Both the timestamp tag and `latest` are pushed to Docker Hub.

**Example tags:**
- `richleigh/dynipupdate:latest` - always the most recent build
- `richleigh/dynipupdate:20251109-143022` - specific build from Nov 9, 2025 at 14:30:22 UTC

## Examples

### Build locally for testing (current platform only)
```bash
make build
# Output: Builds for linux/amd64 (or your current platform)
```

### Build and push multi-platform images
```bash
make build-push
# Output: Builds for all 5 platforms and pushes to Docker Hub
```

### Build locally, then push separately
```bash
make build
# ... test the image locally ...
make push
```

### Override username
```bash
DOCKER_USERNAME=myuser make build-push
```

### Override full image name
```bash
IMAGE_NAME=myorg/myapp make build-push
```

### Preview next version tag
```bash
make version-tag
# Output: Next version tag will be: 20251109-143022
```

## Troubleshooting

**Error: DOCKER_USERNAME not set**

If you see this error, either:
1. Set the `DOCKER_USERNAME` environment variable
2. Override `IMAGE_NAME` directly
3. Ensure you're logged in with `docker login`
