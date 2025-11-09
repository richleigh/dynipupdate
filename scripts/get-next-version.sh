#!/bin/bash
# Get version tag based on current timestamp
# Usage: get-next-version.sh
#
# Returns version in format: YYYYMMDD-HHMMSS
# Example: 20251109-143022

set -euo pipefail

# Generate timestamp-based version
# Format: YYYYMMDD-HHMMSS
VERSION=$(date -u +%Y%m%d-%H%M%S)

echo "$VERSION"
