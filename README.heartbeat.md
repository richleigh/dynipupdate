# DNS Heartbeat System

The DNS heartbeat system provides automatic cleanup of stale DNS records from ephemeral containers and hosts that disappear without gracefully cleaning up their DNS entries.

## Architecture

The system consists of two components:

### 1. Main DNS Updater (runs on each host/container)

When the updater registers internal or combined domain IP addresses, it also creates a **heartbeat TXT record** for each IP:

```
# A record for the IP
internal.example.com A 192.168.1.10

# Heartbeat TXT record
_heartbeat-192-168-1-10.internal.example.com TXT "1699564820,web-container-abc123"
                                                   └─timestamp  └─instance ID
```

The heartbeat contains:
- **Unix timestamp**: When the heartbeat was last updated
- **Instance ID**: Identifier for the host/container (hostname, container name, etc.)

### 2. Cleanup Service (runs once per environment)

A separate service that periodically:
1. Gets all A records for internal/combined domains
2. For each IP, checks its corresponding `_heartbeat-<ip>` TXT record
3. If the heartbeat is missing or older than the threshold (default: 15 min), deletes both the A record and TXT record

## Configuration

### Main Updater

The main updater automatically creates heartbeat records. Configure the instance identifier:

```bash
export INSTANCE_ID="web-container-abc123"  # Defaults to HOSTNAME if not set
```

### Cleanup Service

```bash
# Required
export CF_API_TOKEN="your-cloudflare-api-token"
export CF_ZONE_ID="your-zone-id"

# Which domains to clean up
export INTERNAL_DOMAIN="internal.example.com"  # Optional
export COMBINED_DOMAIN="all.example.com"      # Optional

# Cleanup thresholds
export STALE_THRESHOLD_SECONDS=900   # 15 minutes (default)
export CLEANUP_INTERVAL_SECONDS=300  # 5 minutes (default)
```

## Deployment

**IMPORTANT**: The main updater must run **periodically** (every 5 minutes recommended) to keep the heartbeat alive. Here are deployment options:

### Docker Compose (Recommended for containers)

See `docker-compose.example.yml` for a complete example.

**Main updater** - Runs in a loop every 5 minutes:
```yaml
services:
  dns-updater:
    image: richleigh/dynipupdate:latest
    network_mode: host
    environment:
      - INSTANCE_ID=web-01
      - INTERNAL_DOMAIN=internal.example.com
      - COMBINED_DOMAIN=all.example.com
      - UPDATE_INTERVAL=300  # 5 minutes
    command: >
      sh -c 'while true; do
        /app/dynipupdate;
        sleep ${UPDATE_INTERVAL:-300};
      done'
```

**Cleanup service** - Run **ONCE** per environment (not per host):
```yaml
services:
  dns-cleanup:
    build:
      context: .
      dockerfile: Dockerfile.cleanup
    environment:
      - INTERNAL_DOMAIN=internal.example.com
      - COMBINED_DOMAIN=all.example.com
      - STALE_THRESHOLD_SECONDS=900   # 3x UPDATE_INTERVAL
      - CLEANUP_INTERVAL_SECONDS=300
```

### Bare Metal / VMs (cron)

For bare metal or VMs, use cron to run periodically:

```bash
# Edit crontab
crontab -e

# Add entry to run every 5 minutes
*/5 * * * * INSTANCE_ID=$(hostname) /usr/local/bin/dynipupdate >> /var/log/dynipupdate.log 2>&1
```

### Bare Metal / VMs (systemd timer)

Create systemd service and timer:

**`/etc/systemd/system/dynipupdate.service`:**
```ini
[Unit]
Description=Dynamic DNS Updater
After=network-online.target

[Service]
Type=oneshot
EnvironmentFile=/etc/dynipupdate/config
ExecStart=/usr/local/bin/dynipupdate
```

**`/etc/systemd/system/dynipupdate.timer`:**
```ini
[Unit]
Description=Run Dynamic DNS Updater every 5 minutes

[Timer]
OnBootSec=1min
OnUnitActiveSec=5min

[Install]
WantedBy=timers.target
```

Enable and start:
```bash
systemctl daemon-reload
systemctl enable --now dynipupdate.timer
```

### Kubernetes

For Kubernetes, consider using a DaemonSet for the updater and a Deployment for the cleanup service:

```yaml
# DaemonSet for updater (runs on every node)
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dns-updater
spec:
  template:
    spec:
      hostNetwork: true
      containers:
      - name: dns-updater
        image: richleigh/dynipupdate:latest
        env:
        - name: INSTANCE_ID
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        # ... other env vars

---
# Deployment for cleanup (single replica)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dns-cleanup
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: dns-cleanup
        # Build from Dockerfile.cleanup
        env:
        - name: STALE_THRESHOLD_SECONDS
          value: "900"
        # ... other env vars
```

## How It Works

### Scenario: Container Lifecycle

1. **Container starts**:
   ```
   - Detects IP: 192.168.1.10
   - Creates A record: internal.example.com → 192.168.1.10
   - Creates TXT record: _heartbeat-192-168-1-10.internal.example.com → "1699564820,web-01"
   ```

2. **Container runs (updater runs periodically via cron)**:
   ```
   - Every run updates the TXT record with fresh timestamp
   - Heartbeat stays current
   ```

3. **Container crashes/dies without cleanup**:
   ```
   - A record remains: internal.example.com → 192.168.1.10
   - TXT record stops being updated
   ```

4. **Cleanup service detects stale record**:
   ```
   - Checks _heartbeat-192-168-1-10.internal.example.com
   - Timestamp is 20 minutes old (> 15 min threshold)
   - Deletes both A record and TXT record
   ```

## Benefits

✓ **Stateless**: No volumes or databases needed
✓ **Container-friendly**: Works with ephemeral containers
✓ **Self-documenting**: TXT records show which instance owns which IP
✓ **Automatic cleanup**: Dead containers don't leave stale DNS
✓ **No reachability testing**: Pure DNS-based, no ping/TCP probes

## Tuning

### Stale Threshold

How old a heartbeat can be before cleanup:

- **Too short** (< 5 min): May delete valid but temporarily silent hosts
- **Too long** (> 30 min): Stale records persist longer
- **Recommended**: 2-3x your updater run frequency

Example:
```bash
# If updater runs every 5 minutes
export STALE_THRESHOLD_SECONDS=900  # 15 minutes (3x frequency)
```

### Cleanup Interval

How often the cleanup service checks for stale records:

- **Too frequent** (< 1 min): Wastes API calls, may hit rate limits
- **Too infrequent** (> 15 min): Slower to clean up
- **Recommended**: 5-10 minutes

## Limitations

- Only works for **internal** and **combined** domains (not external IPv4/IPv6)
- Requires periodic updater runs (cron, systemd timer, or loop)
- Cleanup service needs CloudFlare API access

## Example: Multi-Container Environment

```
┌─────────────────────────────────────────────────────────┐
│ Environment with 3 web containers + 1 cleanup service   │
└─────────────────────────────────────────────────────────┘

┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  web-01      │  │  web-02      │  │  web-03      │
│  192.168.1.10│  │  192.168.1.11│  │  192.168.1.12│
│              │  │              │  │              │
│  Runs:       │  │  Runs:       │  │  Runs:       │
│  dns-updater │  │  dns-updater │  │  dns-updater │
│  (cron 5min) │  │  (cron 5min) │  │  (cron 5min) │
└──────────────┘  └──────────────┘  └──────────────┘
       │                  │                  │
       └──────────────────┼──────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │ CloudFlare DNS        │
              │                       │
              │ A records:            │
              │  internal → .10       │
              │  internal → .11       │
              │  internal → .12       │
              │                       │
              │ TXT records:          │
              │  _heartbeat-.10 → ts  │
              │  _heartbeat-.11 → ts  │
              │  _heartbeat-.12 → ts  │
              └───────────────────────┘
                          ▲
                          │
                ┌─────────────────────┐
                │  dns-cleanup        │
                │  (runs every 5min)  │
                │                     │
                │  Checks timestamps  │
                │  Deletes stale IPs  │
                └─────────────────────┘

# What happens when web-02 crashes?
1. web-02 dies (no cleanup)
2. After 15 min, _heartbeat-.11 timestamp is stale
3. Cleanup service deletes both .11 A record and TXT record
4. DNS now only has .10 and .12
```

## Testing

To test the heartbeat system:

1. Start the updater and verify TXT records are created:
   ```bash
   dig _heartbeat-192-168-1-10.internal.example.com TXT
   ```

2. Stop the updater and wait for stale threshold

3. Watch cleanup service logs:
   ```bash
   docker logs -f dns-cleanup
   ```

4. Verify stale records are deleted after threshold expires
