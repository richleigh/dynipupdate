# Build Instructions

This project uses a Makefile with auto-incrementing version tags for Docker builds.

## Quick Start

1. **Set your Docker Hub username:**
   ```bash
   export DOCKER_USERNAME=your-username
   ```

2. **Build and push:**
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
make build       # Run tests, build and push multi-platform images with auto-incrementing tag
make test        # Run Go unit tests only
make version-tag # Show what the next version tag will be
make clean       # Clean build artifacts
make help        # Show help message
```

## Version Tagging

The build system automatically creates version tags in the format `YYYYMMDD###`:

- `YYYYMMDD` - Current date
- `###` - Three-digit incrementing number (001, 002, 003, etc.)

The script queries Docker Hub to find the highest existing tag for today and increments it.

**Example:**
- First build today: `20251109001`
- Second build today: `20251109002`
- Next day's first build: `20251110001`

Both version tag and `latest` are pushed.

## Examples

### Use auto-detected username
```bash
make build
```

### Override username
```bash
DOCKER_USERNAME=myuser make build
```

### Override full image name
```bash
IMAGE_NAME=myorg/myapp make build
```

### Build for specific platforms only
```bash
PLATFORMS=linux/amd64,linux/arm64 make build
```

### Preview next version tag
```bash
make version-tag
# Output: Next version tag will be: 20251109003
```

## Troubleshooting

**Error: DOCKER_USERNAME not set**

If you see this error, either:
1. Set the `DOCKER_USERNAME` environment variable
2. Override `IMAGE_NAME` directly
3. Ensure you're logged in with `docker login`

**Docker Hub API rate limits**

The version detection queries Docker Hub's public API. If you hit rate limits, the script will fall back to version `001`.
