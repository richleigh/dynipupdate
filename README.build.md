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

   Or build without pushing (to test):
   ```bash
   make build
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
make build       # Build multi-platform images (no push)
make build-push  # Build multi-platform images and push to Docker Hub
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

Both the timestamp tag and `latest` are pushed to Docker Hub:
- `your-username/dynipupdate:latest` - Always the most recent build
- `your-username/dynipupdate:20251109-143022` - Specific timestamp for rollback/debugging

Build options:
- `make build` - Builds for all 5 platforms (amd64, arm64, ppc64le, s390x, riscv64) but doesn't push
- `make build-push` - Builds for all platforms AND pushes to Docker Hub with `:latest` and `:YYYYMMDD-HHMMSS` tags

## Examples

### Build without pushing (for testing)
```bash
make build
```

### Build and push with auto-detected username
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

Ensure Docker buildx is properly set up:
```bash
docker buildx ls
```

If you don't see a builder, create one:
```bash
docker buildx create --use
```

## CI/CD Integration

This project includes GitHub Actions for automatic builds when code is merged to main. See [README.ci.md](README.ci.md) for setup instructions.
