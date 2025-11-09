#!/bin/bash
# Get the next version number for a given image and date
# Usage: get-next-version.sh IMAGE_NAME DATE

set -euo pipefail

IMAGE_NAME="${1:-}"
DATE="${2:-}"

if [ -z "$IMAGE_NAME" ] || [ -z "$DATE" ]; then
    echo "Usage: $0 IMAGE_NAME DATE" >&2
    exit 1
fi

# Extract repository name (e.g., "richleigh/dynipupdate" -> "richleigh" and "dynipupdate")
REPO_OWNER=$(echo "$IMAGE_NAME" | cut -d'/' -f1)
REPO_NAME=$(echo "$IMAGE_NAME" | cut -d'/' -f2)

# Try to get tags from Docker Hub API
# Note: Docker Hub API has a limit of 100 results per page, but for daily tags this should be fine
API_URL="https://hub.docker.com/v2/repositories/${REPO_OWNER}/${REPO_NAME}/tags?page_size=100"

# Fetch tags from Docker Hub
TAGS=$(curl -s "$API_URL" 2>/dev/null | grep -o "\"name\":\"[^\"]*\"" | cut -d'"' -f4 || echo "")

# If API call failed or no tags found, start from 001
if [ -z "$TAGS" ]; then
    echo "001"
    exit 0
fi

# Filter tags that match today's date pattern (YYYYMMDD###)
MATCHING_TAGS=$(echo "$TAGS" | grep "^${DATE}[0-9]\{3\}$" || true)

if [ -z "$MATCHING_TAGS" ]; then
    # No tags for today yet, start from 001
    echo "001"
else
    # Find the highest number for today
    HIGHEST=$(echo "$MATCHING_TAGS" | sed "s/^${DATE}//" | sort -n | tail -1)

    # Increment and format as 3-digit number
    NEXT=$((10#$HIGHEST + 1))
    printf "%03d" "$NEXT"
fi
