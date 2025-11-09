#!/bin/bash
# Get the next version number for a given image and date
# Usage: get-next-version.sh IMAGE_NAME DATE
#
# This script uses git tags to track version numbers, avoiding race conditions
# that occur when multiple builds happen simultaneously or when Docker Hub API
# has propagation delays.

set -euo pipefail

IMAGE_NAME="${1:-}"
DATE="${2:-}"

if [ -z "$IMAGE_NAME" ] || [ -z "$DATE" ]; then
    echo "Usage: $0 IMAGE_NAME DATE" >&2
    exit 1
fi

HIGHEST=0

# Fetch all tags from remote to ensure we have the latest
git fetch --tags --quiet 2>/dev/null || true

# Get all git tags matching today's date pattern (v-YYYYMMDD###)
# We use 'v-' prefix to avoid conflicts with other version tags
GIT_TAGS=$(git tag -l "v-${DATE}[0-9][0-9][0-9]" 2>/dev/null || echo "")

if [ -n "$GIT_TAGS" ]; then
    # Extract version numbers and find highest
    HIGHEST=$(echo "$GIT_TAGS" | sed "s/^v-${DATE}//" | sort -n | tail -1)
    HIGHEST=$((10#$HIGHEST))
fi

# Increment and format as 3-digit number
NEXT=$((HIGHEST + 1))
NEXT_VERSION=$(printf "%03d" "$NEXT")

# Create a git tag for this version (this is atomic and will fail if tag exists)
TAG_NAME="v-${DATE}${NEXT_VERSION}"

if git rev-parse "$TAG_NAME" >/dev/null 2>&1; then
    # Tag already exists (race condition caught!)
    # Retry by recursively calling ourselves
    exec "$0" "$IMAGE_NAME" "$DATE"
else
    # Create the tag
    git tag "$TAG_NAME" 2>/dev/null || {
        # Tag creation failed (another process created it first)
        # Retry by recursively calling ourselves
        exec "$0" "$IMAGE_NAME" "$DATE"
    }

    # Push the tag to remote (ignore failures - CI/CD will handle this)
    git push origin "$TAG_NAME" --quiet 2>/dev/null || true
fi

# Output the version number
echo "$NEXT_VERSION"
