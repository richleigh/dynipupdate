# Unified Network Architecture

This guide shows how to build a unified network that seamlessly works across home devices, cloud infrastructure, and mobile devices using dynipupdate, Tailscale, Consul, and Authentik.

## The Problem

You want:
- **Home devices** behind NAT to be accessible from anywhere
- **Cloud servers** to integrate with home infrastructure
- **Mobile devices** to access everything while roaming
- **Service discovery** for microservices and containers
- **Security** with proper authentication and firewall controls
- **No vendor lock-in** - full control over your credentials

## The Solution: Four-Layer Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 4: Identity & Authentication (Authentik)             │
│  - OIDC provider for all services                           │
│  - Self-hosted, you control credentials                     │
│  - SSO for web apps, Tailscale, Consul, etc.                │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Layer 3: Secure Device Mesh (Tailscale)                    │
│  - Device-to-device encrypted tunnels                       │
│  - NAT traversal (no port forwarding needed)                │
│  - ACLs for firewalling between devices                     │
│  - Authenticates against Authentik OIDC                     │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Layer 2: Service Discovery (Consul)                        │
│  - Service registration and DNS                             │
│  - Health checking                                          │
│  - Runs on Tailscale network                                │
│  - Services find each other via *.service.consul            │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Layer 1: Public Gateway (dynipupdate + CloudFlare)         │
│  - Publishes host IPs to CloudFlare DNS                     │
│  - Split-horizon: internal IPs for LAN, external for internet│
│  - Only for public-facing services                          │
│  - Automatic cleanup of stale records                       │
└─────────────────────────────────────────────────────────────┘
```

## How It All Works Together

### Scenario 1: Mobile Device → Home Service

```
Mobile Phone (on cellular)
    ↓
Tailscale encrypted tunnel (via Authentik OIDC auth)
    ↓
Home Server (100.x.x.x Tailscale IP)
    ↓
Consul DNS: webapp.service.consul → 100.64.1.10
    ↓
Web Application Container
```

**Key points:**
- No port forwarding needed
- Encrypted end-to-end via Tailscale
- User authenticated via Authentik
- Tailscale ACLs restrict what mobile can access

### Scenario 2: Internet User → Public Service

```
Internet User
    ↓
DNS query: home.example.com (CloudFlare, via dynipupdate)
    ↓
203.0.113.45 (your static IP)
    ↓
Router port forward (443 → home server)
    ↓
Traefik reverse proxy
    ↓
Authentik login (if required)
    ↓
Backend service (looked up via Consul)
```

**Key points:**
- CloudFlare DNS managed by dynipupdate
- Only specific ports exposed (443 for HTTPS)
- TLS termination at Traefik
- Optional: Require Authentik SSO for web apps

### Scenario 3: LAN Device → Home Service

```
Home Laptop (192.168.1.x)
    ↓
DNS query: home.example.com (CloudFlare, via dynipupdate)
    ↓
192.168.1.10 (internal IP returned via split-horizon DNS)
    ↓
Direct local connection (no internet routing!)
    ↓
Service accessed at LAN speeds
```

**Key points:**
- dynipupdate publishes both internal and external IPs
- Devices on LAN get internal IP from DNS
- No hairpinning through router
- Full gigabit LAN speeds

### Scenario 4: Cloud Server → Home Service

```
Cloud VM (Digital Ocean, AWS, etc.)
    ↓
Tailscale encrypted tunnel
    ↓
Home Server (100.x.x.x Tailscale IP)
    ↓
Consul DNS: database.service.consul → 100.64.1.20
    ↓
PostgreSQL Container
```

**Key points:**
- Cloud and home connected via Tailscale mesh
- Consul provides service discovery across locations
- No public exposure of database
- ACLs restrict cloud → home traffic to specific services

## Component Breakdown

### 1. dynipupdate (Layer 1: Public Gateway)

**Purpose:** Publish host IPs to CloudFlare for public-facing services

**Deploy on:** Physical hosts/VMs that need to be publicly accessible

**Configuration:**
```bash
# .env for a host with public web services
COMBINED_DOMAIN=home.example.com
CF_API_TOKEN=your-cloudflare-token
CF_ZONE_ID=your-zone-id
```

**Result:**
```
home.example.com A 192.168.1.10      # internal IP (for LAN clients)
home.example.com A 203.0.113.45      # external IP (for internet)
home.example.com TXT "1699564820"    # heartbeat
```

**When to use:**
- Web servers you want publicly accessible
- SSH jump hosts
- VPN endpoints
- Public APIs

**When NOT to use:**
- Internal-only services (use Tailscale + Consul instead)
- Databases, internal APIs
- Development services

### 2. Consul (Layer 2: Service Discovery)

**Purpose:** Services register and find each other via DNS

**Deploy on:** Every node (home, cloud, containers)

**Configuration:**
```hcl
# consul.hcl
bind_addr = "{{ GetInterfaceIP \"tailscale0\" }}"  # Bind to Tailscale IP
client_addr = "0.0.0.0"
datacenter = "home"

ui_config {
  enabled = true
}

service {
  name = "web"
  port = 80
  check {
    http = "http://localhost:80/health"
    interval = "10s"
  }
}
```

**Result:**
```bash
# Services auto-register and can be discovered
dig @127.0.0.1 -p 8600 web.service.consul
# Returns: 100.64.1.10 (Tailscale IP of the service)
```

**DNS Integration:**
```bash
# /etc/systemd/resolved.conf
[Resolve]
DNS=127.0.0.1
Domains=~consul
```

Now services can reach each other:
```bash
curl http://webapp.service.consul
psql postgres.service.consul
```

### 3. Tailscale (Layer 3: Secure Mesh)

**Purpose:** Secure, encrypted tunnels between all devices

**Deploy on:** Every device (home servers, cloud VMs, laptops, phones)

**Installation:**
```bash
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --accept-routes
```

**Authentication:** Via Authentik OIDC (see Layer 4)

**ACL Configuration:**
```json
{
  "groups": {
    "group:admins": ["admin@example.com"],
    "group:servers": ["server1@example.com"],
    "group:mobile": ["phone@example.com"]
  },
  "acls": [
    {
      "action": "accept",
      "src": ["group:admins"],
      "dst": ["*:*"]
    },
    {
      "action": "accept",
      "src": ["group:mobile"],
      "dst": ["group:servers:443,22,8080"]
    },
    {
      "action": "accept",
      "src": ["group:servers"],
      "dst": ["group:servers:*"]
    }
  ]
}
```

**Result:**
- Every device gets a stable 100.x.x.x IP
- Encrypted tunnels automatically established
- NAT traversal just works
- ACLs act as distributed firewall

### 4. Authentik (Layer 4: Identity)

**Purpose:** Central authentication and user management

**Deploy on:** One instance (typically a home server or cloud VM)

**Configuration:** See [deploy/authentik/README.md](../deploy/authentik/README.md)

**Published via dynipupdate:**
```bash
# .env for Authentik host
COMBINED_DOMAIN=auth.example.com
```

**Integrations:**
- **Tailscale**: OIDC authentication (users log in via Authentik)
- **Web Apps**: SSO via OIDC/SAML
- **Consul**: (Enterprise) OIDC for UI access
- **Grafana, Portainer, etc.**: SSO

**Result:**
- Users log in once (Authentik)
- Access Tailscale, web apps, services with same credentials
- Central user/group management
- MFA support

## Deployment Walkthrough

### Phase 1: Foundation (dynipupdate)

Get basic DNS working first:

```bash
# On your main home server
git clone https://github.com/richleigh/dynipupdate
cd dynipupdate
cp .env.example .env
nano .env  # Configure CloudFlare credentials

# Test it
docker run --rm --network host --env-file .env richleigh/dynipupdate:latest

# Schedule it (runs every 5 minutes)
echo "*/5 * * * * cd /path/to/dynipupdate && docker run --rm --network host --env-file .env richleigh/dynipupdate:latest" | crontab -
```

**Verify:**
```bash
dig home.example.com
# Should return your IPs
```

### Phase 2: Identity (Authentik)

Set up authentication before adding other services:

```bash
cd deploy/authentik
cp .env.example .env
nano .env  # Fill in secrets and domain

docker-compose up -d
docker-compose logs -f
```

1. Navigate to `http://your-ip:9000/if/flow/initial-setup/`
2. Create admin account
3. Configure reverse proxy with TLS
4. Update dynipupdate to publish auth.example.com

**Verify:**
```bash
curl https://auth.example.com
# Should load Authentik
```

### Phase 3: Secure Mesh (Tailscale + Authentik)

Connect all devices:

```bash
# On each device (home servers, cloud VMs, laptops)
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --accept-routes
```

Configure Tailscale to use Authentik OIDC (see [deploy/authentik/README.md](../deploy/authentik/README.md#integrating-with-tailscale)).

**Verify:**
```bash
tailscale status
# Should show all devices

ping 100.x.x.x  # Ping another device's Tailscale IP
```

### Phase 4: Service Discovery (Consul)

Deploy Consul on Tailscale network:

```bash
# Install Consul
wget https://releases.hashicorp.com/consul/1.17.0/consul_1.17.0_linux_amd64.zip
unzip consul_1.17.0_linux_amd64.zip
sudo mv consul /usr/local/bin/

# Configure (bind to Tailscale interface)
sudo mkdir -p /etc/consul.d
sudo nano /etc/consul.d/consul.hcl
```

```hcl
datacenter = "home"
data_dir = "/opt/consul"
bind_addr = "{{ GetInterfaceIP \"tailscale0\" }}"
client_addr = "0.0.0.0"

ui_config {
  enabled = true
}

server = true
bootstrap_expect = 1
```

```bash
# Start Consul
sudo consul agent -config-dir=/etc/consul.d
```

**Configure DNS forwarding:**
```bash
# /etc/systemd/resolved.conf
[Resolve]
DNS=127.0.0.1
Domains=~consul
```

```bash
sudo systemctl restart systemd-resolved
```

**Verify:**
```bash
dig @127.0.0.1 -p 8600 consul.service.consul
# Should return Consul's IP

# Access UI
curl http://localhost:8500/ui/
```

**Register services:**
```bash
# Example: Register a web service
cat > /etc/consul.d/web.json <<EOF
{
  "service": {
    "name": "web",
    "port": 80,
    "check": {
      "http": "http://localhost:80/health",
      "interval": "10s"
    }
  }
}
EOF

sudo consul reload
```

### Phase 5: Integration

Now everything works together:

1. **Deploy a web app**
   - Runs in Docker container
   - Registers with Consul
   - Accessible via `webapp.service.consul`

2. **Access from mobile**
   - Connect via Tailscale (authenticated via Authentik)
   - `curl http://webapp.service.consul`
   - Works from anywhere

3. **Expose publicly (optional)**
   - Configure reverse proxy (Traefik/Nginx)
   - Point public domain via dynipupdate
   - Require Authentik SSO for access

## Network Topology Examples

### Small Home Setup

```
Internet
   ↓
Router (NAT) - 203.0.113.45
   ↓
Home Server - 192.168.1.10
   ├─ dynipupdate (publishes home.example.com)
   ├─ Authentik (auth.example.com)
   ├─ Consul server
   ├─ Tailscale client (100.64.1.1)
   └─ Docker containers (services registered in Consul)

Laptop - 192.168.1.50
   └─ Tailscale client (100.64.1.2)

Phone (cellular)
   └─ Tailscale app (100.64.1.3)
```

### Home + Cloud Hybrid

```
Home Network (192.168.1.0/24)
   ├─ Home Server 1 - 192.168.1.10
   │    ├─ Authentik
   │    ├─ Consul server
   │    ├─ Tailscale (100.64.1.1)
   │    └─ dynipupdate
   ├─ Home Server 2 - 192.168.1.11
   │    ├─ Consul client
   │    ├─ Tailscale (100.64.1.2)
   │    └─ Docker services
   └─ NAS - 192.168.1.20
        ├─ Consul client
        └─ Tailscale (100.64.1.3)

Cloud (Digital Ocean)
   ├─ Web Server - 54.123.45.67
   │    ├─ Consul client
   │    ├─ Tailscale (100.64.1.10)
   │    └─ dynipupdate (publishes api.example.com)
   └─ Database - 54.123.45.68
        ├─ Consul client
        └─ Tailscale (100.64.1.11)

Mobile Devices
   ├─ Phone (100.64.1.20)
   └─ Laptop (100.64.1.21)
```

**Traffic flows:**
- **Public web traffic** → CloudFlare DNS → Cloud Web Server (port 443)
- **Home devices** → Tailscale mesh → any service
- **Cloud ↔ Home** → Tailscale mesh → Consul service discovery
- **Services find each other** → Consul DNS (*.service.consul)

## Security Best Practices

### Defense in Depth

```
Layer 1: Tailscale ACLs
   ├─ Only allow specific ports between groups
   ├─ Mobile devices: client-only (no inbound)
   └─ Servers: restricted to specific services

Layer 2: Host Firewalls (iptables/nftables)
   ├─ Allow Tailscale interface (100.x.x.x)
   ├─ Allow LAN (192.168.x.x)
   ├─ Drop everything else by default
   └─ Only specific public ports (443, 22)

Layer 3: Application Auth
   ├─ Require Authentik SSO for web apps
   ├─ Use strong passwords/MFA (hardware keys recommended)
   ├─ Hardware security keys (YubiKey) for admin accounts
   └─ Regular access reviews

Layer 4: Network Segmentation
   ├─ Separate VLANs for IoT devices
   ├─ Consul service mesh (optional mTLS)
   └─ Separate Tailscale networks for prod/dev
```

### Firewall Rules Example

```bash
# Home server firewall (iptables)
# Allow established connections
iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

# Allow localhost
iptables -A INPUT -i lo -j ACCEPT

# Allow Tailscale
iptables -A INPUT -i tailscale0 -j ACCEPT

# Allow LAN
iptables -A INPUT -s 192.168.1.0/24 -j ACCEPT

# Allow public HTTPS and SSH (for published services)
iptables -A INPUT -p tcp --dport 443 -j ACCEPT
iptables -A INPUT -p tcp --dport 22 -j ACCEPT

# Drop everything else
iptables -P INPUT DROP
```

### Authentik Security

- **Enable MFA for all admin accounts** (hardware keys strongly recommended)
- **Use hardware security keys** (YubiKey) - see [YubiKey Setup Guide](YUBIKEY_SETUP.md)
  - Buy 2 keys minimum (primary + backup)
  - Set up TOTP and recovery codes as backup methods
  - Never rely on single authentication method
- Use strong passwords (generated)
- Regular backups of PostgreSQL database
- Keep Authentik updated
- Use HTTPS only (reverse proxy with valid cert)

### Tailscale Security

- Enable key expiry
- Use Authentik OIDC (not direct signup)
- Restrict ACLs to least privilege
- Disable key reuse
- Monitor device list regularly

### Consul Security

- Enable ACLs (Consul Enterprise/Community)
- Bind to Tailscale interface only
- Use TLS for agent communication
- Regular backups
- Restrict UI access (use Authentik SSO if Enterprise)

## Monitoring and Observability

### Metrics to Track

**dynipupdate:**
- Heartbeat freshness
- DNS update success rate
- IP change frequency

**Tailscale:**
- Connection status
- Latency between nodes
- Failed auth attempts

**Consul:**
- Service health checks
- Leader elections (if clustered)
- DNS query rates

**Authentik:**
- Login attempts (success/failure)
- Active sessions
- MFA enrollment rate

### Monitoring Stack

```
Prometheus (metrics collection)
   ↓
Grafana (dashboards)
   ↓
Loki (logs)
   ↓
Alertmanager (alerts)
```

Deploy on Tailscale network, accessible to admins only.

## Troubleshooting

### Can't reach service from mobile

1. **Check Tailscale connection:**
   ```bash
   tailscale status
   # Ensure mobile device is connected
   ```

2. **Check ACLs:**
   - View in Tailscale admin console
   - Ensure mobile group can reach target ports

3. **Check service health in Consul:**
   ```bash
   dig @127.0.0.1 -p 8600 myservice.service.consul
   ```

4. **Check firewall on target host:**
   ```bash
   sudo iptables -L -n -v
   ```

### DNS not resolving correctly

1. **Check dynipupdate is running:**
   ```bash
   docker ps | grep dynipupdate
   ```

2. **Check CloudFlare DNS:**
   ```bash
   dig @1.1.1.1 home.example.com
   ```

3. **Check heartbeat:**
   ```bash
   dig @1.1.1.1 TXT home.example.com
   # Should return recent timestamp
   ```

4. **Check split-horizon:**
   ```bash
   # From LAN:
   dig home.example.com
   # Should return 192.168.x.x

   # From internet:
   dig home.example.com @8.8.8.8
   # Should return public IP
   ```

### Tailscale auth not working

1. **Check Authentik OIDC config:**
   - Redirect URI exactly matches: `https://login.tailscale.com/a/oauth_response`
   - Issuer URL includes trailing slash
   - Client ID/secret correct

2. **Check Authentik is accessible:**
   ```bash
   curl https://auth.example.com/.well-known/openid-configuration
   # Should return OIDC discovery document
   ```

3. **Check Authentik logs:**
   ```bash
   cd deploy/authentik
   docker-compose logs -f server
   ```

### Consul services not registering

1. **Check Consul agent is running:**
   ```bash
   consul members
   ```

2. **Check service definition:**
   ```bash
   consul catalog services
   consul catalog nodes -service=myservice
   ```

3. **Check health checks:**
   ```bash
   consul monitor
   # Watch for health check failures
   ```

## Advanced Topics

### Multi-Datacenter Consul

For home + cloud setups, consider Consul federation:

```hcl
# Home datacenter
datacenter = "home"
primary_datacenter = "home"

# Cloud datacenter
datacenter = "cloud"
primary_datacenter = "home"
primary_gateways = ["100.64.1.1:8443"]  # Tailscale IP of home
```

Services in cloud can discover services in home and vice versa.

### Consul Connect (Service Mesh)

Add automatic mTLS between services:

```hcl
connect {
  enabled = true
}
```

Services communicate via encrypted sidecars, even on untrusted networks.

### Split-Horizon DNS with Consul

Use Consul's prepared queries for location-aware routing:

```json
{
  "Name": "webapp",
  "Service": {
    "Service": "webapp",
    "Failover": {
      "Datacenters": ["cloud"]
    },
    "OnlyPassing": true,
    "Near": "_agent"
  }
}
```

Queries return nearest healthy instance.

### dynipupdate in Kubernetes

Deploy as DaemonSet to update DNS for each node:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dynipupdate
spec:
  template:
    spec:
      hostNetwork: true
      containers:
      - name: dynipupdate
        image: richleigh/dynipupdate:latest
        env:
        - name: COMBINED_DOMAIN
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
```

## Cost Analysis

### Self-Hosted (This Architecture)

**One-time:**
- Domain: $10-15/year
- (Optional) VPS for Authentik: $5/month

**Recurring:**
- CloudFlare: Free tier sufficient
- Tailscale: Free for personal use (up to 100 devices)
- Authentik: $0 (self-hosted)
- Consul: $0 (Community Edition)
- dynipupdate: $0 (open source)

**Total: ~$5-10/month** (if you need a VPS, otherwise $0)

### Commercial Alternatives

- **Auth0/Okta**: $30-100/month
- **Cloudflare Tunnels**: Free but vendor lock-in
- **Tailscale Teams**: $6/user/month
- **Managed Consul**: $100+/month

**Self-hosted saves you $50-200/month** while giving you full control.

## Next Steps

1. **Start with dynipupdate** - Get basic DNS working
2. **Add Authentik** - Set up authentication
3. **Deploy Tailscale** - Connect all devices
4. **Install Consul** - Service discovery
5. **Integrate everything** - Connect the layers

Each component works independently, so you can deploy incrementally.

## Additional Resources

- [dynipupdate README](../README.md)
- [Authentik Deployment Guide](../deploy/authentik/README.md)
- [YubiKey Setup Guide](YUBIKEY_SETUP.md) - Hardware security keys with backup strategies
- [Tailscale Documentation](https://tailscale.com/kb/)
- [Consul Documentation](https://www.consul.io/docs)
- [CloudFlare API Docs](https://developers.cloudflare.com/api/)

## Contributing

Found this architecture useful? Contributions welcome:
- Additional deployment examples
- Terraform/Ansible automation
- Integration guides for other services
- Security improvements

Open an issue or pull request!
