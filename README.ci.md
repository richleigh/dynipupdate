# GitHub Actions CI/CD Setup

This project uses GitHub Actions to automatically build and push Docker images when changes are merged to the main branch.

## How It Works

When a PR is merged to `main` or `master`:
1. GitHub Actions runs the test suite (`make test`)
2. Builds multi-platform Docker images for 5 architectures
3. Pushes to Docker Hub with tags:
   - `:latest` (always the most recent build)
   - `:YYYYMMDD-HHMMSS` (specific timestamp for this build)

## Setup Instructions

### 1. Create a Docker Hub Access Token

**Why a token instead of your password?**
- Tokens can be revoked without changing your password
- Tokens can have limited permissions
- More secure for CI/CD environments

**Steps:**
1. Log in to [Docker Hub](https://hub.docker.com/)
2. Click your username → **Account Settings**
3. Go to **Security** → **New Access Token**
4. Name it something like `github-actions-dynipupdate`
5. Set permissions: **Read & Write** (or **Read, Write, Delete** if you want CI to clean up old images)
6. Click **Generate**
7. **Copy the token** - you won't be able to see it again!

### 2. Add GitHub Secrets

1. Go to your GitHub repository
2. Click **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**

Add these two secrets:

| Secret Name | Value | Description |
|-------------|-------|-------------|
| `DOCKER_USERNAME` | Your Docker Hub username | e.g., `richleigh` |
| `DOCKER_TOKEN` | The access token from step 1 | The token you just generated |

### 3. Verify It Works

1. Create a test PR with a small change
2. Merge the PR to main
3. Go to **Actions** tab in GitHub
4. Watch the "Build and Push Docker Images" workflow run
5. Check [Docker Hub](https://hub.docker.com/) to see the new images

## Workflow Details

The workflow is defined in `.github/workflows/docker-build-push.yml`:

```yaml
on:
  push:
    branches:
      - main
      - master
  workflow_dispatch:  # Manual trigger
```

**Triggers:**
- Automatic: Push to `main` or `master` (when PRs are merged)
- Manual: Click "Run workflow" in the Actions tab

## Manual Workflow Dispatch

You can manually trigger a build without merging a PR:

1. Go to **Actions** tab
2. Select "Build and Push Docker Images"
3. Click **Run workflow**
4. Select the branch
5. Click **Run workflow**

This is useful for:
- Rebuilding after a Docker Hub issue
- Creating a new build from an older commit
- Testing the CI/CD pipeline

## Local Development

For local testing and development, continue using the Makefile:

```bash
# Build locally (current platform only, no push)
make build

# Build and push multi-platform
export DOCKER_USERNAME=your-username
make build-push
```

See [README.build.md](README.build.md) for full build instructions.

## Troubleshooting

### Workflow fails with "denied: requested access to the resource is denied"

**Problem:** Docker Hub credentials are incorrect or missing.

**Fix:**
1. Verify `DOCKER_USERNAME` secret matches your Docker Hub username exactly
2. Verify `DOCKER_TOKEN` is a valid access token (not your password)
3. Check the token hasn't expired
4. Ensure the token has Read & Write permissions

### Workflow fails with "Error: DOCKER_USERNAME not set"

**Problem:** GitHub Secret `DOCKER_USERNAME` is not set.

**Fix:**
1. Go to Settings → Secrets and variables → Actions
2. Add `DOCKER_USERNAME` secret with your Docker Hub username

### Tests pass locally but fail in CI

**Problem:** Environment differences or missing dependencies.

**Fix:**
1. Check the workflow uses the correct Go version (`go-version: '1.21'`)
2. Verify all dependencies are in `go.mod`
3. Check for platform-specific assumptions in tests

### Multi-platform build fails

**Problem:** Docker Buildx not properly configured.

**Fix:**
The workflow uses `docker/setup-buildx-action@v3` which should handle this automatically. If it still fails:
1. Check Docker Buildx version in the workflow logs
2. Try updating the action version in the workflow file

## Security Notes

1. **Never commit secrets** - Use GitHub Secrets, never hardcode tokens
2. **Rotate tokens regularly** - Create new Docker Hub tokens periodically
3. **Use minimal permissions** - Docker Hub tokens should only have Read & Write
4. **Monitor access logs** - Check Docker Hub for unexpected activity
5. **Revoke compromised tokens immediately** - Better safe than sorry

## Viewing Build Results

After a successful build:

1. **GitHub Actions Summary** shows published image tags
2. **Docker Hub** shows the new images at `https://hub.docker.com/r/your-username/dynipupdate/tags`
3. Pull the latest with:
   ```bash
   docker pull your-username/dynipupdate:latest
   ```

## Cost Considerations

**GitHub Actions:** Free for public repositories (2,000 minutes/month for private)

**Docker Hub:**
- Free tier: Unlimited public repositories
- Rate limits: 200 pulls per 6 hours (authenticated), 100 pulls per 6 hours (anonymous)
- Image retention: Images not pulled for 6 months may be deleted on free tier

For heavy usage, consider upgrading to Docker Hub Pro ($5/month) for unlimited pulls.
