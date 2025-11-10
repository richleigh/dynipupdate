# Dynamic DNS Updater for CloudFlare

A lightweight Go-based dynamic DNS updater that automatically detects and updates CloudFlare DNS records with automatic cleanup of stale records.

## Features

- **Ultra-lightweight**: Built with Go and UPX compressed for minimal Docker image size (~2-3 MB)
- **Automatic IP detection** for internal (RFC1918) and external (IPv4/IPv6) addresses
- **Heartbeat system** with automatic cleanup of stale DNS records from dead containers/hosts
- **Single binary** with both update and cleanup modes
- CloudFlare API integration for DNS record management
- Static binary with no runtime dependencies
- Docker containerization for easy deployment
- Configuration via environment variables

## IP Detection Methods

- **Internal IPv4**: Scans network interfaces for RFC1918 private IP addresses
- **External IPv4**: Queries multiple services via IPv4 DNS (ipify, icanhazip, etc.)
- **External IPv6**: Queries multiple services via IPv6 DNS

## Configuration

All configuration is done via environment variables. See `.env.example` for a complete list.

### Required Variables

| Variable | Description |
|----------|-------------|
| `CF_API_TOKEN` | CloudFlare API token (create at https://dash.cloudflare.com/profile/api-tokens) |
| `CF_ZONE_ID` | CloudFlare Zone ID (found in domain overview) |
| `INTERNAL_DOMAIN` | Full domain for internal IPv4 records (e.g., `anubis.i.4.bees.wtf`) |
| `EXTERNAL_DOMAIN` | Full domain for external IPv4 record (e.g., `anubis.e.4.bees.wtf`) |
| `IPV6_DOMAIN` | Full domain for external IPv6 record (e.g., `anubis.6.bees.wtf`) |
| `COMBINED_DOMAIN` | **Main domain** - aggregates ALL IPs (e.g., `anubis.bees.wtf`) - **use this!** |
| `TOP_LEVEL_DOMAIN` | **Optional** - CNAME alias pointing to COMBINED_DOMAIN (e.g., `anubis.example.com`) |

**Why COMBINED_DOMAIN?** This is the killer feature - one domain that resolves to all your IPs:
- From your LAN: resolves to internal IPs (192.168.x.x, 10.x.x.x, 172.16.x.x)
- From the internet: resolves to external IPv4 and IPv6
- Your OS/browser automatically picks the best route

**Why TOP_LEVEL_DOMAIN?** Optional friendly alias via CNAME:
- Points to COMBINED_DOMAIN (e.g., `anubis.example.com` -> `anubis.bees.wtf`)
- Users can use the friendly name, DNS resolves through CNAME to get all IPs
- Also gets a heartbeat TXT record for automatic cleanup
- Example:
  ```
  # Combined domain with actual IPs
  anubis.bees.wtf A 192.168.1.10
  anubis.bees.wtf AAAA 2001:db8::1
  anubis.bees.wtf TXT "1699564820"

  # Top-level CNAME alias
  anubis.example.com CNAME anubis.bees.wtf
  anubis.example.com TXT "1699564820"
  ```

### Optional Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CF_PROXIED` | Proxy through CloudFlare (true/false) | `false` |
| `STALE_THRESHOLD_SECONDS` | Cleanup: Age before records are stale | `3600` (1 hour) |
| `CLEANUP_INTERVAL_SECONDS` | Cleanup: How often to check | `300` (5 minutes) |

## Usage

### Update Mode (Default)

Run once to update DNS records with current IPs:

```bash
docker run --rm --env-file .env dynipupdate
```

Typically scheduled with cron:
```bash
*/5 * * * * docker run --rm --env-file /path/to/.env dynipupdate
```

### Usage Examples

**Simple setup (just combined domain):**
```bash
# .env configuration
COMBINED_DOMAIN=anubis.bees.wtf

# Results in DNS:
anubis.bees.wtf A 192.168.1.10        # internal IP
anubis.bees.wtf A 203.0.113.45        # external IP
anubis.bees.wtf AAAA 2001:db8::1      # external IPv6
anubis.bees.wtf TXT "1699564820"      # heartbeat
```

**With friendly CNAME alias:**
```bash
# .env configuration
COMBINED_DOMAIN=anubis.bees.wtf
TOP_LEVEL_DOMAIN=anubis.example.com

# Results in DNS:
anubis.bees.wtf A 192.168.1.10
anubis.bees.wtf A 203.0.113.45
anubis.bees.wtf AAAA 2001:db8::1
anubis.bees.wtf TXT "1699564820"

anubis.example.com CNAME anubis.bees.wtf  # friendly alias
anubis.example.com TXT "1699564820"       # also gets heartbeat

# Users can now use either name:
ssh anubis.example.com    # resolves via CNAME
ssh anubis.bees.wtf       # resolves directly
```

**Full setup (all domains):**
```bash
# .env configuration
INTERNAL_DOMAIN=anubis.i.4.bees.wtf
EXTERNAL_DOMAIN=anubis.e.4.bees.wtf
IPV6_DOMAIN=anubis.6.bees.wtf
COMBINED_DOMAIN=anubis.bees.wtf
TOP_LEVEL_DOMAIN=anubis.example.com

# Results in separate purpose-specific domains plus combined
```

### Cleanup Mode

Run as a long-running service to automatically remove stale DNS records:

```bash
docker run --rm --env-file .env dynipupdate -cleanup
```

The cleanup service:
- Monitors heartbeat TXT records created by the updater
- Deletes DNS records when heartbeats are missing or stale
- Runs continuously, checking at `CLEANUP_INTERVAL_SECONDS`

**Deploy ONCE per environment** (not per host). The cleanup service monitors all records.

## Docker Deployment

### Using docker-compose

```yaml
version: '3.8'
services:
  # DNS updater - run on every host/container
  dns-updater:
    image: richleigh/dynipupdate:latest
    network_mode: host
    env_file: .env
    restart: always
    entrypoint: sh -c 'while true; do /dynipupdate; sleep 300; done'

  # DNS cleanup - run ONCE per environment
  dns-cleanup:
    image: richleigh/dynipupdate:latest
    env_file: .env
    restart: always
    command: ["-cleanup"]
```

## How Heartbeat Cleanup Works

When the updater runs, it creates/updates a heartbeat TXT record **at the same domain name** as the A/AAAA records:

```
# Multiple A records for the IPs
anubis.i.4.bees.wtf A 192.168.1.10
anubis.i.4.bees.wtf A 192.168.1.11

# ONE heartbeat TXT record (same name, different type)
anubis.i.4.bees.wtf TXT "1699564820"
```

The heartbeat TXT record contains:
- **Unix timestamp**: When the updater last ran (e.g., 1699564820)
- Format: `"timestamp"` (quoted string)

**How it works:**
1. Each time the updater runs, it updates the TXT record with the current timestamp
2. The cleanup service scans **all TXT records in the zone** to discover heartbeats
3. For each heartbeat, it checks if the timestamp is stale (default: older than 1 hour)
4. If stale, cleanup deletes ALL records for that domain (A/AAAA/CNAME/TXT)
5. This automatically weeds out dead processes/containers hanging around for no good reason

**Key features:**
- **Automatic discovery**: Cleanup doesn't need to know domain names in advance
- **Zone-wide scanning**: Finds all domains with heartbeats across your entire zone
- **Keeps DNS clean**: Any host that stops updating its heartbeat gets cleaned up
- **Un-sanctioned removal**: Domains without valid heartbeats are automatically removed
- **CNAME cleanup**: Top-level CNAME aliases are also removed when stale

## Building

### Multi-Platform Docker Build

```bash
# Set your Docker Hub username
export DOCKER_USERNAME=your-username

# Build and push multi-platform images
make build-push
```

Supports: `linux/amd64`, `linux/arm64`, `linux/ppc64le`, `linux/s390x`, `linux/riscv64`

### Version Tags

Images are tagged with:
- `:latest` - Most recent build
- `:YYYYMMDD-HHMMSS` - Git commit timestamp

### Build Targets

```bash
make build       # Build images (no push)
make push        # Push previously built images
make build-push  # Build and push in one step
make test        # Run Go unit tests
make clean       # Clean build artifacts
```

### Local Go Build

```bash
go build -o dynipupdate main.go
./dynipupdate        # Update mode
./dynipupdate -cleanup  # Cleanup mode
```

## CloudFlare API Token Setup

1. Go to https://dash.cloudflare.com/profile/api-tokens
2. Click "Create Token"
3. Use the "Edit zone DNS" template or create a custom token with:
   - Permissions: `Zone > DNS > Edit`
   - Zone Resources: `Include > Specific zone > your-domain.com`
4. Copy the generated token and use it as `CF_API_TOKEN`

## GitHub Actions CI/CD

This project includes GitHub Actions for automatic multi-platform builds on merge to main.

### Setup

1. Create a Docker Hub access token at https://hub.docker.com/
2. Add GitHub Secrets:
   - `DOCKER_USERNAME`: Your Docker Hub username
   - `DOCKER_TOKEN`: The access token from step 1
3. Merge PRs to main - images are automatically built and pushed

Workflow runs:
- On push to `main` or `master`
- Manually via "Run workflow" in Actions tab

## Scheduling

Run the updater periodically to keep DNS records fresh:

### Cron (Linux/macOS)
```bash
*/5 * * * * docker run --rm --env-file /path/to/.env dynipupdate
```

### systemd Timer (Linux)

Create `/etc/systemd/system/dynipupdate.timer`:
```ini
[Unit]
Description=Dynamic DNS Updater Timer

[Timer]
OnBootSec=1min
OnUnitActiveSec=5min

[Install]
WantedBy=timers.target
```

Create `/etc/systemd/system/dynipupdate.service`:
```ini
[Unit]
Description=Dynamic DNS Updater

[Service]
Type=oneshot
EnvironmentFile=/path/to/.env
ExecStart=/usr/local/bin/dynipupdate
```

Enable:
```bash
sudo systemctl enable --now dynipupdate.timer
```

### macOS Launch Agent

Create `~/Library/LaunchAgents/com.user.dynipupdate.plist`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.dynipupdate</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/dynipupdate</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>CF_API_TOKEN</key>
        <string>your_token_here</string>
        <!-- Add other env vars -->
    </dict>
    <key>StartInterval</key>
    <integer>300</integer>
</dict>
</plist>
```

Load:
```bash
launchctl load ~/Library/LaunchAgents/com.user.dynipupdate.plist
```

## Troubleshooting

### No Internal IPv4 Detected
- Ensure the host has a network interface with an RFC1918 address
- For Docker: use `--network host` mode

### No External IPv4/IPv6 Detected
- Verify internet connectivity
- Check DNS resolution works
- For IPv6: ensure the host has IPv6 connectivity

### CloudFlare API Errors
- Verify API token has correct permissions
- Check Zone ID is correct
- Ensure the domain is active in CloudFlare

## Exit Codes

- `0`: All updates successful
- `1`: Some or all updates failed

## License

MIT License - feel free to use and modify as needed.
