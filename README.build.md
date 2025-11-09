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
make build       # Build images (default: all platforms, no push)
make push        # Push previously built images to Docker Hub
make build-push  # Build and push in one step (default: all platforms)
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

Build workflow:
- `make build` - Builds for specified platforms (default: all 5) but doesn't push
- `make push` - Pushes what you already built
- `make build-push` - Builds and pushes in one step (default: all platforms)

This is useful for debugging platform-specific issues. You can build just one platform, test it, then push only that platform if it works.

## Examples

### Build all platforms (no push)
```bash
make build
```

### Build and push all platforms in one step
```bash
make build-push
```

### Debug a specific platform
```bash
# Build just amd64 to test
PLATFORMS=linux/amd64 make build

# If it works, push it
PLATFORMS=linux/amd64 make push

# Or build just arm64
PLATFORMS=linux/arm64 make build
```

### Build and push specific platforms only
```bash
# Only build and push amd64 and arm64
PLATFORMS=linux/amd64,linux/arm64 make build-push
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

**Multi-platform build fails**

Ensure Docker buildx is properly set up:
```bash
docker buildx ls
```

If you don't see a builder, create one:
```bash
docker buildx create --use
```

**Platform-specific build issues**

If one platform fails, you can build and test each platform individually:

```bash
# Test each platform separately
PLATFORMS=linux/amd64 make build
PLATFORMS=linux/arm64 make build
PLATFORMS=linux/ppc64le make build
PLATFORMS=linux/s390x make build
PLATFORMS=linux/riscv64 make build

# Push only the platforms that work
PLATFORMS=linux/amd64,linux/arm64 make push
```

## CI/CD Integration

This project includes GitHub Actions for automatic builds when code is merged to main. See [README.ci.md](README.ci.md) for setup instructions.
