# Dynamic DNS Updater for CloudFlare

A lightweight Go-based dynamic DNS updater that automatically detects and updates CloudFlare DNS records with three types of IP addresses:

1. **Internal IPv4** - RFC1918 private addresses (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
2. **External IPv4** - Public-facing IPv4 address detected via DNS TXT query
3. **External IPv6** - Public-facing IPv6 address detected via DNS TXT query

## Features

- **Ultra-lightweight**: Built with Go and UPX compressed for minimal Docker image size (~2-3 MB)
- Automatic IP address detection for all three types
- CloudFlare API integration for DNS record management
- Creates or updates DNS records as needed
- Deletes stale DNS records when IP addresses are no longer available
- Static binary with no runtime dependencies
- Docker containerization for easy deployment
- Configuration via environment variables
- Comprehensive logging

## How It Works

### IP Detection Methods

- **Internal IPv4**: Scans network interfaces for RFC1918 private IP addresses
- **External IPv4**: Queries `o-o.myaddr.l.google.com` TXT record via IPv4 DNS servers
- **External IPv6**: Queries `o-o.myaddr.l.google.com` TXT record via IPv6 DNS servers

### CloudFlare Integration

The script uses the CloudFlare REST API to:
1. Check if DNS records exist
2. Create new records if they don't exist
3. Update existing records with current IP addresses

## Requirements

- Docker (for containerized deployment) or Go 1.21+ (for building from source)
- CloudFlare account with API token
- Network connectivity for IPv4 and/or IPv6 (as needed)

## Configuration

All configuration is done via environment variables. See `.env.example` for a complete list.

### Required Variables

| Variable | Description |
|----------|-------------|
| `CF_API_TOKEN` | CloudFlare API token (create at https://dash.cloudflare.com/profile/api-tokens) |
| `CF_ZONE_ID` | CloudFlare Zone ID (found in domain overview) |
| `INTERNAL_DOMAIN` | Subdomain for internal IPv4 records only (e.g., `host.internal`) |
| `EXTERNAL_DOMAIN` | Subdomain for external IPv4 record only (e.g., `host.external`) |
| `IPV6_DOMAIN` | Subdomain for external IPv6 record only (e.g., `host.ipv6`) |
| `COMBINED_DOMAIN` | **Main domain** - aggregates ALL IPs (e.g., `host`) - **use this!** |

**Important:** Do NOT include your zone name (e.g., `.bees.wtf`) in domain variables. CloudFlare automatically appends it.

**Why COMBINED_DOMAIN?** This is the killer feature - one domain that resolves to all your IPs:
- From your LAN: resolves to internal IPs (192.168.x.x, 10.x.x.x, 172.16.x.x)
- From the internet: resolves to external IPv4 and IPv6
- Your OS/browser automatically picks the best route

### Optional Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `INSTANCE_ID` | Identifier for heartbeat records | Machine hostname |
| `CF_PROXIED` | Proxy through CloudFlare (true/false) | `false` |

## Usage

### Docker Deployment (Recommended)

1. Build the Docker image:
```bash
docker build -t dynip-updater .
```

2. Run the container with environment variables:
```bash
docker run --rm \
  -e CF_API_TOKEN=your_token \
  -e CF_ZONE_ID=your_zone_id \
  -e INTERNAL_DOMAIN=host.internal.example.com \
  -e EXTERNAL_DOMAIN=host.external.example.com \
  -e IPV6_DOMAIN=host.ipv6.example.com \
  -e COMBINED_DOMAIN=host.example.com \
  dynip-updater
```

**Note:** Do NOT include your zone name (e.g., `.bees.wtf`). If your zone is `bees.wtf`:
```bash
  -e INTERNAL_DOMAIN=anubis.i.4      # Creates: anubis.i.4.bees.wtf
  -e EXTERNAL_DOMAIN=anubis.e.4      # Creates: anubis.e.4.bees.wtf
  -e IPV6_DOMAIN=anubis.6            # Creates: anubis.6.bees.wtf
  -e COMBINED_DOMAIN=anubis          # Creates: anubis.bees.wtf (use this!)
```

Then `ssh anubis.bees.wtf` works from anywhere!

3. Or use an environment file:
```bash
# Create .env file from example
cp .env.example .env
# Edit .env with your values
nano .env

# Run with env file
docker run --rm --env-file .env dynip-updater
```

### Scheduled Updates with Docker

Use cron or a scheduler to run periodically:

```bash
# Run every 5 minutes via cron
*/5 * * * * docker run --rm --env-file /path/to/.env dynip-updater
```

Or use docker-compose with a restart policy:

```yaml
version: '3.8'
services:
  dynip-updater:
    build: .
    env_file: .env
    restart: always
    # Run every 5 minutes
    entrypoint: |
      sh -c 'while true; do /dynip-updater; sleep 300; done'
```

### Direct Go Usage

1. Build the binary:
```bash
go build -o dynip-updater main.go
```

2. Set environment variables:
```bash
export CF_API_TOKEN=your_token
export CF_ZONE_ID=your_zone_id
export INTERNAL_DOMAIN=host.internal.example.com
export EXTERNAL_DOMAIN=host.external.example.com
export IPV6_DOMAIN=host.ipv6.example.com
export COMBINED_DOMAIN=host.example.com  # The one you'll actually use!
```

3. Run the binary:
```bash
./dynip-updater
```

## CloudFlare API Token Setup

1. Go to https://dash.cloudflare.com/profile/api-tokens
2. Click "Create Token"
3. Use the "Edit zone DNS" template or create a custom token with:
   - Permissions: `Zone > DNS > Edit`
   - Zone Resources: `Include > Specific zone > your-domain.com`
4. Copy the generated token and use it as `CF_API_TOKEN`

## Finding Your Zone ID

1. Log in to CloudFlare dashboard
2. Select your domain
3. Scroll down on the Overview page
4. Find "Zone ID" in the API section on the right sidebar

## Logging

The script outputs detailed logs including:
- IP addresses detected
- DNS records created/updated
- Any errors encountered

Log levels:
- `INFO`: Normal operation
- `WARNING`: Non-critical issues (e.g., no IPv6 connectivity)
- `ERROR`: Critical failures

## Exit Codes

- `0`: All updates successful
- `1`: Some or all updates failed

## Troubleshooting

### No Internal IPv4 Detected
- Ensure the host has a network interface with an RFC1918 address
- Check that the container has access to host networking if needed

### No External IPv4/IPv6 Detected
- Verify internet connectivity
- Check DNS resolution works
- For IPv6: ensure the host has IPv6 connectivity

### CloudFlare API Errors
- Verify API token has correct permissions
- Check Zone ID is correct
- Ensure the domain is active in CloudFlare

### Docker Networking Issues
- For internal IP detection, may need `--network host` mode:
```bash
docker run --rm --network host --env-file .env dynip-updater
```

## License

MIT License - feel free to use and modify as needed.

## Contributing

Issues and pull requests welcome!
