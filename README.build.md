# Build Instructions

This project uses a Makefile with timestamp-based version tags for Docker builds.

## Quick Start

1. **Set your Docker Hub username:**
   ```bash
   export DOCKER_USERNAME=your-username
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
| `DOCKER_USERNAME` | Your Docker Hub username | Auto-detected from `~/.docker/config.json` |
| `DOCKER_REPO` | Repository name | `dynipupdate` |
| `IMAGE_NAME` | Full image name (overrides above) | `${DOCKER_USERNAME}/${DOCKER_REPO}` |
| `PLATFORMS` | Target platforms | `linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/riscv64` |

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

The build system automatically creates version tags based on UTC timestamps in the format `YYYYMMDD-HHMMSS`:

- `YYYYMMDD` - Current date (UTC)
- `HHMMSS` - Current time (UTC)

This ensures every build has a unique, sortable version tag without coordination between build servers.

**Examples:**
- Build at 2:30:22 PM UTC on Nov 9, 2025: `20251109-143022`
- Build at 8:15:05 AM UTC on Nov 10, 2025: `20251110-081505`

Both the timestamp tag and `latest` are created:
- `your-username/dynipupdate:latest` - Always the most recent build
- `your-username/dynipupdate:20251109-143022` - Specific timestamp for rollback/debugging

## Build Workflows

### Local Development (Single Platform)

For quick local testing without pushing to Docker Hub:

```bash
make build
```

This builds only for your current platform (e.g., `linux/amd64`) and loads the image locally. Perfect for testing before publishing.

### Production Build (Multi-Platform)

For publishing to Docker Hub with multi-platform support:

```bash
export DOCKER_USERNAME=your-username
make build-push
```

This builds for all 5 platforms and pushes both `:latest` and `:YYYYMMDD-HHMMSS` tags.

### Build Then Push Separately

For more control over the process:

```bash
# Build and test locally
make build

# Test your image
docker run --rm your-username/dynipupdate:latest

# If everything works, push to Docker Hub
make push
```

Note: `make build` only builds for your current platform, so `make push` will only push that single platform. For multi-platform, use `make build-push`.

## Examples

### Use auto-detected username
```bash
make build-push
```

### Override username
```bash
DOCKER_USERNAME=myuser make build-push
```

### Override full image name
```bash
IMAGE_NAME=myorg/myapp make build-push
```

### Build for specific platforms only
```bash
PLATFORMS=linux/amd64,linux/arm64 make build-push
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

**Multi-platform build fails**

The `make build` target only builds for your current platform because Docker buildx cannot load multi-platform images locally. Use `make build-push` for multi-platform builds, which pushes directly to Docker Hub.

**Image not found for `make push`**

You need to run `make build` first to create the local images before pushing them.

## CI/CD Integration

This project includes GitHub Actions for automatic builds when code is merged to main. See [README.ci.md](README.ci.md) for setup instructions.
