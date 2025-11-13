#!/bin/bash
set -euo pipefail

# Certificate Manager for Dynamic DNS Setup
# Automatically generates wildcard TLS certificates based on BEES_IP_UPDATE_* domains
# Integrates with acme.sh for Let's Encrypt certificate management

#############################################################################
# Configuration
#############################################################################

# Base domain configuration - defines where the "root" domain starts
# Format: DOMAIN:LEVELS
# - LEVELS: number of labels in the base domain (e.g., bees.wtf=2, bees.co.uk=3)
#
# Examples:
#   bees.wtf:2           → base is "bees.wtf"
#   bees.co.uk:3         → base is "bees.co.uk"
#   example.com:2        → base is "example.com"
#
# You can configure multiple base domains separated by spaces
BASE_DOMAIN_CONFIG="${BEES_CERT_BASE_DOMAINS:-bees.wtf:2}"

# DNS provider for acme.sh (used for DNS-01 challenge)
# See https://github.com/acmesh-official/acme.sh/wiki/dnsapi for options
DNS_PROVIDER="${BEES_CERT_DNS_PROVIDER:-dns_cf}"

# Certificate output directory
CERT_DIR="${BEES_CERT_DIR:-/etc/ssl/bees}"

# acme.sh installation directory
ACME_HOME="${BEES_CERT_ACME_HOME:-$HOME/.acme.sh}"

# Dry run mode - set to 1 to just print commands without executing
DRY_RUN="${BEES_CERT_DRY_RUN:-0}"

# Email for Let's Encrypt notifications
ACME_EMAIL="${BEES_CERT_EMAIL:-}"

# Heartbeat checking - skip wildcard patterns where all hosts are stale
# Set to 1 to enable heartbeat checking (default: enabled)
CHECK_HEARTBEATS="${BEES_CERT_CHECK_HEARTBEATS:-1}"

# Stale threshold in seconds - how old a heartbeat can be before considered stale
# Default: 3600 seconds (1 hour)
STALE_THRESHOLD="${BEES_IP_UPDATE_STALE_THRESHOLD_SECONDS:-3600}"

# DNS server to query for heartbeats (empty = use system default)
DNS_SERVER="${BEES_CERT_DNS_SERVER:-}"

#############################################################################
# Functions
#############################################################################

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*" >&2
}

error() {
    log "ERROR: $*"
    exit 1
}

# Extract base domain from a full domain based on level configuration
# Args: $1=full_domain (e.g., anubis.i.4.bees.wtf), $2=base_domain (e.g., bees.wtf), $3=levels
get_base_domain() {
    local full_domain="$1"
    local base_domain="$2"

    # Check if this domain ends with the configured base
    if [[ "$full_domain" == *"$base_domain" ]]; then
        echo "$base_domain"
        return 0
    fi

    return 1
}

# Extract subdomain pattern from a full domain
# Args: $1=full_domain, $2=base_domain
# Returns: subdomain pattern (e.g., "i.4" from "anubis.i.4.bees.wtf" with base "bees.wtf")
get_subdomain_pattern() {
    local full_domain="$1"
    local base_domain="$2"

    # Remove the base domain and the hostname
    local without_base="${full_domain%.$base_domain}"

    # If it's just the hostname (e.g., "anubis"), return empty (will use *.base)
    if [[ "$without_base" != *.* ]]; then
        echo ""
        return 0
    fi

    # Remove the first label (hostname) to get the pattern
    local pattern="${without_base#*.}"
    echo "$pattern"
}

# Generate wildcard domain from pattern
# Args: $1=pattern, $2=base_domain
get_wildcard() {
    local pattern="$1"
    local base_domain="$2"

    if [[ -z "$pattern" ]]; then
        echo "*.$base_domain"
    else
        echo "*.$pattern.$base_domain"
    fi
}

# Query DNS TXT record for a domain
# Args: $1=domain
# Returns: TXT record content (without quotes) or empty if not found
query_txt_record() {
    local domain="$1"
    local dns_args=()

    if [[ -n "$DNS_SERVER" ]]; then
        dns_args+=("@$DNS_SERVER")
    fi

    # Use dig if available, otherwise try host
    if command -v dig &>/dev/null; then
        dig "${dns_args[@]}" +short TXT "$domain" 2>/dev/null | tr -d '"' | head -1
    elif command -v host &>/dev/null; then
        host -t TXT "$domain" ${DNS_SERVER:+"$DNS_SERVER"} 2>/dev/null | \
            grep -oP '"\K[^"]+' | head -1
    else
        log "WARNING: Neither 'dig' nor 'host' command found - cannot query DNS"
        return 1
    fi
}

# Parse heartbeat timestamp from TXT record
# Args: $1=txt_content
# Returns: timestamp (Unix epoch) or empty if invalid
parse_heartbeat() {
    local txt="$1"

    # Heartbeat format is just a Unix timestamp (possibly quoted)
    # Remove any quotes and whitespace
    txt=$(echo "$txt" | tr -d '" \t\n\r')

    # Validate it's a number
    if [[ "$txt" =~ ^[0-9]+$ ]]; then
        echo "$txt"
    fi
}

# Check if a heartbeat is fresh (not stale)
# Args: $1=timestamp (Unix epoch)
# Returns: 0 if fresh, 1 if stale
is_heartbeat_fresh() {
    local timestamp="$1"
    local now=$(date +%s)
    local age=$((now - timestamp))

    if [[ $age -le $STALE_THRESHOLD ]]; then
        return 0  # Fresh
    else
        return 1  # Stale
    fi
}

# Check if a domain has a fresh heartbeat
# Args: $1=domain
# Returns: 0 if fresh, 1 if stale or no heartbeat
has_fresh_heartbeat() {
    local domain="$1"

    # Query TXT record
    local txt
    txt=$(query_txt_record "$domain") || return 1

    if [[ -z "$txt" ]]; then
        return 1  # No heartbeat found
    fi

    # Parse timestamp
    local timestamp
    timestamp=$(parse_heartbeat "$txt") || return 1

    if [[ -z "$timestamp" ]]; then
        return 1  # Invalid heartbeat
    fi

    # Check freshness
    is_heartbeat_fresh "$timestamp"
}

# Parse all BEES_IP_UPDATE_* domains from environment
get_all_domains() {
    local domains=()

    # Get all environment variables with BEES_IP_UPDATE_ prefix
    while IFS='=' read -r key value; do
        # Skip if not our prefix
        [[ "$key" =~ ^BEES_IP_UPDATE_ ]] || continue

        # Skip if not a domain variable
        [[ "$key" =~ DOMAIN$ ]] || continue

        # Skip empty values
        [[ -n "$value" ]] || continue

        domains+=("$value")
    done < <(env)

    # Return unique domains
    printf '%s\n' "${domains[@]}" | sort -u
}

# Find which base domain config matches a given domain
# Returns: base_domain:levels or empty if no match
find_base_config() {
    local domain="$1"

    for config in $BASE_DOMAIN_CONFIG; do
        local base_domain="${config%:*}"
        local levels="${config#*:}"

        if [[ "$domain" == *"$base_domain" ]]; then
            echo "$config"
            return 0
        fi
    done

    return 1
}

# Generate wildcard patterns from discovered domains
generate_wildcards() {
    local -A wildcard_to_domains   # Maps wildcard -> space-separated list of domains
    local -A wildcard_has_fresh    # Maps wildcard -> 1 if at least one domain is fresh
    local -A base_domains

    log "Discovering domains from BEES_IP_UPDATE_* environment variables..."

    if [[ "$CHECK_HEARTBEATS" == "1" ]]; then
        log "Heartbeat checking: ENABLED (stale threshold: ${STALE_THRESHOLD}s)"
    else
        log "Heartbeat checking: DISABLED"
    fi

    log ""

    while IFS= read -r domain; do
        [[ -n "$domain" ]] || continue

        log "  Found: $domain"

        # Find matching base domain configuration
        local config
        if ! config=$(find_base_config "$domain"); then
            log "    WARNING: No base domain configuration matches '$domain' - skipping"
            continue
        fi

        local base_domain="${config%:*}"
        local levels="${config#*:}"

        # Track base domains we've seen
        base_domains["$base_domain"]=1

        # Get subdomain pattern
        local pattern
        pattern=$(get_subdomain_pattern "$domain" "$base_domain")

        # Generate wildcard
        local wildcard
        wildcard=$(get_wildcard "$pattern" "$base_domain")

        # Track which domains belong to this wildcard
        if [[ -z "${wildcard_to_domains[$wildcard]:-}" ]]; then
            wildcard_to_domains["$wildcard"]="$domain"
        else
            wildcard_to_domains["$wildcard"]+=" $domain"
        fi

        # Check heartbeat if enabled
        local is_fresh=1  # Default: assume fresh if not checking
        if [[ "$CHECK_HEARTBEATS" == "1" ]]; then
            if has_fresh_heartbeat "$domain"; then
                is_fresh=1
                log "    Heartbeat: FRESH ✓"
            else
                is_fresh=0
                log "    Heartbeat: STALE or missing ✗"
            fi
        fi

        # Mark wildcard as having at least one fresh domain
        if [[ $is_fresh -eq 1 ]]; then
            wildcard_has_fresh["$wildcard"]=1
        fi

        if [[ -n "$pattern" ]]; then
            log "    Pattern: $pattern → Wildcard: $wildcard"
        else
            log "    Top-level → Wildcard: $wildcard"
        fi
    done < <(get_all_domains)

    log ""

    # Filter out wildcards where all domains are stale
    local -a valid_wildcards=()
    for wildcard in "${!wildcard_to_domains[@]}"; do
        if [[ "$CHECK_HEARTBEATS" == "1" ]] && [[ -z "${wildcard_has_fresh[$wildcard]:-}" ]]; then
            log "  SKIPPING wildcard $wildcard - all domains are stale:"
            for domain in ${wildcard_to_domains[$wildcard]}; do
                log "    - $domain (stale)"
            done
        else
            valid_wildcards+=("$wildcard")
        fi
    done

    # Add base domains themselves (always include these)
    for base in "${!base_domains[@]}"; do
        valid_wildcards+=("$base")
        log "  Added base domain: $base"
    done

    # Return all valid unique wildcards
    printf '%s\n' "${valid_wildcards[@]}" | sort -u
}

# Generate acme.sh command
generate_acme_command() {
    local wildcards=()

    # Read wildcards into array
    while IFS= read -r wildcard; do
        wildcards+=("$wildcard")
    done

    if [[ ${#wildcards[@]} -eq 0 ]]; then
        error "No wildcards to request. Check your BEES_IP_UPDATE_*_DOMAIN environment variables."
    fi

    log ""
    log "Generating certificate request for ${#wildcards[@]} domain pattern(s)..."

    # Build acme.sh command
    local cmd="$ACME_HOME/acme.sh --issue --dns $DNS_PROVIDER"

    # Add email if configured
    if [[ -n "$ACME_EMAIL" ]]; then
        cmd="$cmd --accountemail $ACME_EMAIL"
    fi

    # Add each domain
    for wildcard in "${wildcards[@]}"; do
        cmd="$cmd -d '$wildcard'"
    done

    # Add cert directory
    cmd="$cmd --cert-file $CERT_DIR/cert.pem"
    cmd="$cmd --key-file $CERT_DIR/key.pem"
    cmd="$cmd --ca-file $CERT_DIR/ca.pem"
    cmd="$cmd --fullchain-file $CERT_DIR/fullchain.pem"

    echo "$cmd"
}

# Install certificate to output directory
install_cert() {
    local wildcards=("$@")

    log "Installing certificate to $CERT_DIR..."

    # Create cert directory if it doesn't exist
    mkdir -p "$CERT_DIR"

    # Use first wildcard as the main domain for acme.sh
    local main_domain="${wildcards[0]}"

    "$ACME_HOME/acme.sh" --install-cert -d "$main_domain" \
        --cert-file "$CERT_DIR/cert.pem" \
        --key-file "$CERT_DIR/key.pem" \
        --ca-file "$CERT_DIR/ca.pem" \
        --fullchain-file "$CERT_DIR/fullchain.pem"
}

#############################################################################
# Main
#############################################################################

main() {
    log "=== BEES Certificate Manager ==="
    log "Base domain config: $BASE_DOMAIN_CONFIG"
    log "DNS provider: $DNS_PROVIDER"
    log "Certificate directory: $CERT_DIR"
    log "acme.sh home: $ACME_HOME"

    if [[ "$DRY_RUN" == "1" ]]; then
        log "DRY RUN MODE - no certificates will be requested"
    fi

    log ""

    # Check if acme.sh is installed
    if [[ ! -f "$ACME_HOME/acme.sh" ]]; then
        error "acme.sh not found at $ACME_HOME/acme.sh - please install it first"
    fi

    # Generate wildcard list
    local wildcards
    wildcards=$(generate_wildcards)

    if [[ -z "$wildcards" ]]; then
        error "No domains found. Please set BEES_IP_UPDATE_*_DOMAIN environment variables."
    fi

    log ""
    log "=== Certificate Request Summary ==="
    log "The following wildcard patterns will be requested:"
    echo "$wildcards" | while read -r wc; do
        log "  - $wc"
    done

    # Generate command
    local cmd
    cmd=$(echo "$wildcards" | generate_acme_command)

    log ""
    log "=== acme.sh Command ==="
    echo "$cmd"

    if [[ "$DRY_RUN" == "1" ]]; then
        log ""
        log "DRY RUN - command not executed"
        log "To execute, set BEES_CERT_DRY_RUN=0"
        exit 0
    fi

    log ""
    log "=== Executing Certificate Request ==="

    # Execute the command
    if eval "$cmd"; then
        log "Certificate request successful!"
    else
        error "Certificate request failed"
    fi
}

# Show help
if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]]; then
    cat <<EOF
BEES Certificate Manager

Automatically generates wildcard TLS certificates for all domains configured
in BEES_IP_UPDATE_* environment variables.

USAGE:
    $0 [--help]

ENVIRONMENT VARIABLES:
    BEES_CERT_BASE_DOMAINS    Base domain configuration (default: bees.wtf:2)
                              Format: domain:levels [domain2:levels2 ...]
                              Examples:
                                bees.wtf:2
                                bees.wtf:2 bees.co.uk:3

    BEES_CERT_DNS_PROVIDER    DNS provider for acme.sh (default: dns_cf)
                              See: https://github.com/acmesh-official/acme.sh/wiki/dnsapi

    BEES_CERT_DIR             Certificate output directory (default: /etc/ssl/bees)

    BEES_CERT_ACME_HOME       acme.sh installation directory (default: ~/.acme.sh)

    BEES_CERT_DRY_RUN         Set to 1 for dry-run mode (default: 0)

    BEES_CERT_EMAIL           Email for Let's Encrypt notifications

REQUIRED:
    Your existing BEES_IP_UPDATE_* environment variables must be set.
    Example:
        BEES_IP_UPDATE_INTERNAL_DOMAIN=anubis.i.4.bees.wtf
        BEES_IP_UPDATE_COMBINED_DOMAIN=anubis.any.bees.wtf

EXAMPLES:
    # Dry run to see what would be requested
    BEES_CERT_DRY_RUN=1 $0

    # Request certificates for bees.wtf domains
    BEES_CERT_EMAIL=admin@bees.wtf $0

    # Use multiple base domains
    BEES_CERT_BASE_DOMAINS="bees.wtf:2 bees.co.uk:3" $0

EOF
    exit 0
fi

main "$@"
