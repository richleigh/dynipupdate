# Build Instructions

This project uses a Makefile with auto-incrementing version tags for Docker builds.

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
   make build
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

### How Version Incrementing Works

To avoid race conditions when multiple builds happen simultaneously (e.g., CI/CD pipelines, multiple developers), the system uses **git tags** to track versions:

1. **Fetches** existing git tags from remote (format: `v-YYYYMMDD###`)
2. **Finds** the highest version number for today
3. **Creates** a new git tag atomically (if tag exists, retry automatically)
4. **Pushes** the git tag to remote
5. **Builds** and pushes Docker image with matching tag

This approach ensures:
- **No duplicate versions** - git tag creation is atomic
- **Works with multiple build servers** - git is the source of truth
- **No API propagation delays** - doesn't depend on Docker Hub API
- **Automatic retry** - handles concurrent builds gracefully

**Example:**
- First build today: `20251109001` (creates git tag `v-20251109001`)
- Second build today: `20251109002` (creates git tag `v-20251109002`)
- Next day's first build: `20251110001` (creates git tag `v-20251110001`)

Both Docker image version tag and `latest` are pushed.

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

**Duplicate version tags**

If you see the same version number used twice, this indicates the git tags weren't being pushed correctly. The system creates git tags (format: `v-YYYYMMDD###`) to track versions. Ensure:
- You have push access to the repository
- Your git remote is configured correctly
- Run `git fetch --tags` before building to get the latest tags

**Git tags vs Docker tags**

The build system creates two types of tags:
- **Git tags** (format: `v-YYYYMMDD###`) - used internally for version tracking
- **Docker tags** (format: `YYYYMMDD###`) - the actual Docker image tags

The git tags ensure version uniqueness across builds and servers.
