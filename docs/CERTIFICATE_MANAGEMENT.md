# Certificate Management for Dynamic DNS

The `certmanager.sh` script automatically generates wildcard TLS certificates for all your configured domains using Let's Encrypt and acme.sh.

## Overview

When running dynamic DNS with multiple hosts and subdomains, manually requesting certificates for each domain becomes tedious. This script:

1. **Discovers** all domains from your `BEES_IP_UPDATE_*` environment variables
2. **Generates** wildcard patterns to cover all hosts (e.g., `*.i.4.bees.wtf`)
3. **Checks** heartbeat freshness to skip stale subdomain patterns
4. **Requests** a single certificate covering all active patterns

## Quick Start

### Prerequisites

1. Install [acme.sh](https://github.com/acmesh-official/acme.sh):
   ```bash
   curl https://get.acme.sh | sh
   ```

2. Configure DNS API credentials for your provider:
   ```bash
   # For CloudFlare
   export CF_Token="your-cloudflare-api-token"
   export CF_Account_ID="your-account-id"
   export CF_Zone_ID="your-zone-id"
   ```

3. Set your `BEES_IP_UPDATE_*` environment variables (same as dynipupdate)

### Basic Usage

```bash
# Dry run to see what would be requested
BEES_CERT_DRY_RUN=1 ./certmanager.sh

# Request certificates
BEES_CERT_EMAIL=admin@bees.wtf ./certmanager.sh
```

## How It Works

### Wildcard Pattern Generation

Given these domains:
```bash
BEES_IP_UPDATE_INTERNAL_DOMAIN=anubis.i.4.bees.wtf
BEES_IP_UPDATE_EXTERNAL_DOMAIN=anubis.e.4.bees.wtf
BEES_IP_UPDATE_IPV6_DOMAIN=anubis.6.bees.wtf
BEES_IP_UPDATE_IPV4_RANGE_1_DOMAIN=anubis.ts.vpn.bees.wtf
BEES_IP_UPDATE_COMBINED_DOMAIN=anubis.any.bees.wtf
BEES_IP_UPDATE_TOP_LEVEL_DOMAIN=anubis.bees.wtf
```

The script generates these wildcards:
```
*.i.4.bees.wtf       # Covers anubis.i.4.bees.wtf, osiris.i.4.bees.wtf, etc.
*.e.4.bees.wtf       # Covers anubis.e.4.bees.wtf, osiris.e.4.bees.wtf, etc.
*.6.bees.wtf         # Covers anubis.6.bees.wtf, osiris.6.bees.wtf, etc.
*.ts.vpn.bees.wtf    # Covers anubis.ts.vpn.bees.wtf, osiris.ts.vpn.bees.wtf, etc.
*.any.bees.wtf       # Covers anubis.any.bees.wtf, osiris.any.bees.wtf, etc.
*.bees.wtf           # Covers anubis.bees.wtf, osiris.bees.wtf, etc.
bees.wtf             # The base domain itself
```

**Result**: One certificate covers **all current and future hosts** in these patterns!

### Heartbeat Checking

By default, the script checks heartbeat TXT records for each domain to determine if hosts are still active.

**Behavior**:
- If **all** hosts in a subdomain pattern are stale → pattern is **skipped**
- If **at least one** host in a pattern is fresh → pattern is **included**

**Example**:
```bash
anubis.i.4.bees.wtf      # Heartbeat: fresh (30 minutes old)
osiris.i.4.bees.wtf      # Heartbeat: stale (2 hours old)

Result: *.i.4.bees.wtf is INCLUDED (anubis is still fresh)
```

```bash
anubis.ts.vpn.bees.wtf   # Heartbeat: stale (5 hours old)
osiris.ts.vpn.bees.wtf   # Heartbeat: stale (10 hours old)

Result: *.ts.vpn.bees.wtf is SKIPPED (all hosts are stale)
```

This prevents requesting certificates for abandoned subdomain patterns.

## Configuration

All configuration uses environment variables:

### Required

| Variable | Description | Default |
|----------|-------------|---------|
| `BEES_IP_UPDATE_*_DOMAIN` | Your domain configurations (same as dynipupdate) | *Required* |

### Optional

| Variable | Description | Default |
|----------|-------------|---------|
| `BEES_CERT_BASE_DOMAINS` | Base domain configuration (see below) | `bees.wtf:2` |
| `BEES_CERT_DNS_PROVIDER` | DNS provider for acme.sh | `dns_cf` |
| `BEES_CERT_DIR` | Certificate output directory | `/etc/ssl/bees` |
| `BEES_CERT_ACME_HOME` | acme.sh installation directory | `~/.acme.sh` |
| `BEES_CERT_EMAIL` | Email for Let's Encrypt notifications | *(empty)* |
| `BEES_CERT_DRY_RUN` | Dry run mode (1=enabled, 0=disabled) | `0` |
| `BEES_CERT_CHECK_HEARTBEATS` | Check heartbeat freshness (1=enabled, 0=disabled) | `1` |
| `BEES_CERT_DNS_SERVER` | DNS server to query for heartbeats | *(system default)* |
| `BEES_IP_UPDATE_STALE_THRESHOLD_SECONDS` | Heartbeat staleness threshold | `3600` (1 hour) |

### Base Domain Configuration

The base domain configuration defines where your "root" domain starts. This is needed to correctly parse subdomain patterns.

**Format**: `domain:levels`

**Examples**:
```bash
# bees.wtf has 2 labels (bees + wtf)
BEES_CERT_BASE_DOMAINS="bees.wtf:2"

# bees.co.uk has 3 labels (bees + co + uk)
BEES_CERT_BASE_DOMAINS="bees.co.uk:3"

# Multiple base domains (space-separated)
BEES_CERT_BASE_DOMAINS="bees.wtf:2 bees.co.uk:3"
```

**Why this matters**:
- `anubis.i.4.bees.wtf` with base `bees.wtf:2` → pattern is `i.4` → wildcard is `*.i.4.bees.wtf` ✓
- `anubis.i.4.bees.co.uk` with base `bees.co.uk:3` → pattern is `i.4` → wildcard is `*.i.4.bees.co.uk` ✓
- Wrong config would generate incorrect wildcards

## Usage Examples

### Example 1: Standard Setup

```bash
#!/bin/bash
# setup-certs.sh

export BEES_CERT_EMAIL=admin@bees.wtf
export BEES_CERT_DIR=/etc/ssl/bees

# Run certmanager (uses existing BEES_IP_UPDATE_* vars)
./certmanager.sh
```

### Example 2: Multiple Base Domains

```bash
#!/bin/bash

# Configure multiple base domains
export BEES_CERT_BASE_DOMAINS="bees.wtf:2 bees.co.uk:3"

# Configure domains
export BEES_IP_UPDATE_INTERNAL_DOMAIN=anubis.i.4.bees.wtf
export BEES_IP_UPDATE_EXTERNAL_DOMAIN=anubis.e.4.bees.co.uk

# Request certificates
./certmanager.sh
```

### Example 3: Disable Heartbeat Checking

```bash
#!/bin/bash

# Disable heartbeat checking (always include all patterns)
export BEES_CERT_CHECK_HEARTBEATS=0

./certmanager.sh
```

### Example 4: Custom Stale Threshold

```bash
#!/bin/bash

# Consider heartbeats stale after 2 hours instead of 1 hour
export BEES_IP_UPDATE_STALE_THRESHOLD_SECONDS=7200

./certmanager.sh
```

### Example 5: Using Different DNS Provider

```bash
#!/bin/bash

# Use Route53 instead of CloudFlare
export BEES_CERT_DNS_PROVIDER=dns_aws

# Configure AWS credentials
export AWS_ACCESS_KEY_ID=your-key-id
export AWS_SECRET_ACCESS_KEY=your-secret

./certmanager.sh
```

See [acme.sh DNS API docs](https://github.com/acmesh-official/acme.sh/wiki/dnsapi) for all supported providers.

## Certificate Renewal

### Manual Renewal

acme.sh automatically installs a cron job for renewals, but you can manually renew:

```bash
~/.acme.sh/acme.sh --renew-all
```

### Automatic Renewal with Heartbeat Checking

To automatically skip stale patterns during renewal, run certmanager regularly:

```bash
# Add to crontab (run daily at 3 AM)
0 3 * * * /path/to/certmanager.sh >> /var/log/certmanager.log 2>&1
```

This will:
1. Check heartbeats for all domains
2. Request new certificate if any patterns change
3. Skip stale subdomain patterns automatically

## Troubleshooting

### No Wildcards Generated

**Symptom**: Script says "No domains found"

**Solution**: Ensure your `BEES_IP_UPDATE_*_DOMAIN` environment variables are set:
```bash
env | grep BEES_IP_UPDATE_
```

### DNS Challenge Fails

**Symptom**: acme.sh fails with "DNS challenge failed"

**Possible causes**:
1. DNS API credentials not set correctly
2. Wrong DNS provider configured
3. DNS propagation delay

**Solution**:
```bash
# Check credentials are set
env | grep CF_

# Test DNS API manually
~/.acme.sh/acme.sh --issue --dns dns_cf --test -d test.bees.wtf
```

### Heartbeat Check Fails

**Symptom**: All domains shown as stale even though they're active

**Possible causes**:
1. DNS not propagated yet
2. Wrong DNS server being queried
3. Firewall blocking DNS queries

**Solution**:
```bash
# Test DNS query manually
dig TXT anubis.bees.wtf

# Disable heartbeat checking temporarily
BEES_CERT_CHECK_HEARTBEATS=0 ./certmanager.sh
```

### Wildcard Pattern Mismatch

**Symptom**: Generated wildcards don't match your domain structure

**Solution**: Check your base domain configuration:
```bash
# For bees.wtf (2 labels)
BEES_CERT_BASE_DOMAINS="bees.wtf:2"

# For bees.co.uk (3 labels)
BEES_CERT_BASE_DOMAINS="bees.co.uk:3"
```

## Security Considerations

### Wildcard Certificate Risk

Wildcard certificates (`*.i.4.bees.wtf`) can be used for **any** hostname in that pattern. If compromised:

**Risk**: Attacker could create valid certificates for `malicious.i.4.bees.wtf`

**Mitigation**:
1. Protect private key files (chmod 600)
2. Use separate patterns for sensitive vs. public hosts
3. Monitor certificate transparency logs
4. Rotate certificates regularly

### Heartbeat Privacy

Heartbeat TXT records are public DNS records. They reveal:
- Which hosts are active
- When they last checked in
- Your domain structure

**If this is a concern**: Disable heartbeat checking and manage patterns manually.

## Integration with dynipupdate

The certmanager is designed to work seamlessly with dynipupdate:

1. **Same environment variables**: Uses your existing `BEES_IP_UPDATE_*` config
2. **Same heartbeat system**: Reads the same TXT records for staleness detection
3. **Automatic pattern discovery**: No manual certificate domain lists needed

**Recommended workflow**:

1. Configure your domains in `.env`
2. Run `dynipupdate` to create DNS records + heartbeats
3. Run `certmanager.sh` to request certificates covering all domains
4. Add both to cron for automatic updates

## Advanced Usage

### Custom Certificate Deployment

After issuance, certificates are in `$BEES_CERT_DIR`:

```bash
/etc/ssl/bees/
├── cert.pem         # Certificate only
├── key.pem          # Private key
├── ca.pem           # CA certificate
└── fullchain.pem    # Certificate + CA chain
```

**Example nginx configuration**:
```nginx
server {
    listen 443 ssl;
    server_name anubis.i.4.bees.wtf;

    ssl_certificate /etc/ssl/bees/fullchain.pem;
    ssl_certificate_key /etc/ssl/bees/key.pem;

    # ... rest of config
}
```

### Selective Pattern Inclusion

To request certificates for only specific patterns, filter domains before running:

```bash
#!/bin/bash

# Only export specific domains
export BEES_IP_UPDATE_COMBINED_DOMAIN=anubis.any.bees.wtf
export BEES_IP_UPDATE_TOP_LEVEL_DOMAIN=anubis.bees.wtf

# Run certmanager (only these patterns will be included)
./certmanager.sh
```

## See Also

- [dynipupdate README](../README.md) - Main dynamic DNS documentation
- [acme.sh documentation](https://github.com/acmesh-official/acme.sh/wiki) - Certificate issuance
- [Let's Encrypt rate limits](https://letsencrypt.org/docs/rate-limits/) - Important to know
