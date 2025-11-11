# Authentik OIDC Provider Deployment

This directory contains the deployment configuration for Authentik, a self-hosted identity provider (IdP) that provides OIDC/OAuth2 authentication for your services.

## Why Authentik?

Authentik gives you full control over your credentials and user management while providing:
- **OIDC/OAuth2 provider** for Tailscale, web apps, and other services
- **Modern web UI** for user and application management
- **Multi-factor authentication** support
- **LDAP provider** (optional) for legacy applications
- **Self-hosted** - your credentials, your infrastructure

## Architecture

```
Internet/Mobile
    ↓
CloudFlare DNS (via dynipupdate)
    ↓
Your Static IP → Reverse Proxy (Traefik/Nginx)
    ↓
Authentik (auth.yourdomain.com)
    ↓
    ├─→ Tailscale (OIDC authentication)
    ├─→ Web Applications (SSO)
    ├─→ Grafana, Portainer, etc. (SSO)
    └─→ Consul (optional ACL integration)
```

## Quick Start

### 1. Prerequisites

- Docker and docker-compose installed
- A domain configured in CloudFlare
- dynipupdate running to publish this host's IP

### 2. Generate Secrets

```bash
# Generate PostgreSQL password
openssl rand -base64 32

# Generate Authentik secret key
openssl rand -base64 60
```

### 3. Configure Environment

```bash
cd deploy/authentik
cp .env.example .env
nano .env  # Fill in your generated secrets and domain
```

Required variables:
- `PG_PASS` - PostgreSQL database password (generated above)
- `AUTHENTIK_SECRET_KEY` - Authentik encryption key (generated above)
- `AUTHENTIK_DOMAIN` - Your domain (e.g., auth.example.com)

Optional (can configure later):
- Email settings for password resets

### 4. Start Authentik

```bash
docker-compose up -d
```

Check logs:
```bash
docker-compose logs -f
```

### 5. Initial Setup

1. **Access Authentik**: Navigate to `http://your-ip:9000/if/flow/initial-setup/`
   - Create your admin account (do this immediately!)

2. **Configure DNS**: Add Authentik to your dynipupdate configuration
   ```bash
   # In your dynipupdate .env
   COMBINED_DOMAIN=auth.example.com
   ```

3. **Set up Reverse Proxy** (recommended):
   - Use Traefik, Nginx, or Caddy in front of Authentik
   - Terminate TLS at the reverse proxy
   - Forward to Authentik on port 9000

## Integrating with Tailscale

### Step 1: Create OIDC Application in Authentik

1. Log into Authentik admin interface
2. Navigate to **Applications** → **Providers** → **Create**
3. Select **OAuth2/OpenID Provider**
4. Configure:
   - **Name**: Tailscale
   - **Authorization flow**: default-provider-authorization-implicit-consent
   - **Client type**: Confidential
   - **Client ID**: (auto-generated, save this!)
   - **Client Secret**: (auto-generated, save this!)
   - **Redirect URIs**:
     ```
     https://login.tailscale.com/a/oauth_response
     ```
   - **Scopes**: `openid`, `email`, `profile`
   - **Subject mode**: Based on User's Email
   - **Include claims in id_token**: ✓ (checked)

5. Click **Create**

### Step 2: Create Application Entry

1. Navigate to **Applications** → **Applications** → **Create**
2. Configure:
   - **Name**: Tailscale
   - **Slug**: tailscale
   - **Provider**: (select the provider you just created)
   - **Launch URL**: `https://login.tailscale.com/`

3. Click **Create**

### Step 3: Configure Tailscale

1. Go to [Tailscale Admin Console](https://login.tailscale.com/admin/)
2. Navigate to **Settings** → **OAuth Clients**
3. Click **Use a custom OIDC provider**
4. Fill in:
   - **Issuer URL**: `https://auth.example.com/application/o/tailscale/`
     (replace auth.example.com with your domain)
   - **Client ID**: (from Authentik)
   - **Client Secret**: (from Authentik)

5. Click **Save**

### Step 4: Test Login

1. Log out of Tailscale on a device
2. Run: `tailscale up`
3. You should be redirected to your Authentik login page
4. Log in with your Authentik credentials
5. Success! Your device is now authenticated via your OIDC provider

## User Management

### Creating Users

1. Navigate to **Directory** → **Users** → **Create**
2. Fill in:
   - **Username**: user's email or username
   - **Email**: user's email address
   - **Name**: Full name
   - **Password**: Set initial password (they can change it)

3. Click **Create**

### Creating Groups

Groups help manage permissions and ACLs:

1. Navigate to **Directory** → **Groups** → **Create**
2. Example groups:
   - `admins` - Full access to everything
   - `servers` - Access to server resources
   - `mobile` - Limited access for mobile devices
   - `home` - Home network devices

3. Assign users to groups
4. Use groups in Tailscale ACLs and application policies

## Tailscale ACL Integration

Once users authenticate via Authentik, you can use their groups in Tailscale ACLs:

```json
{
  "groups": {
    "group:admins": ["user@example.com"],
    "group:users": ["otheruser@example.com"]
  },
  "acls": [
    {
      "action": "accept",
      "src": ["group:admins"],
      "dst": ["*:*"]
    },
    {
      "action": "accept",
      "src": ["group:users"],
      "dst": ["tag:servers:443,22"]
    }
  ]
}
```

## Securing Authentik

### 1. Use HTTPS

**Always** put a reverse proxy with TLS in front of Authentik:

```yaml
# Example Traefik labels
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.authentik.rule=Host(`auth.example.com`)"
  - "traefik.http.routers.authentik.entrypoints=websecure"
  - "traefik.http.routers.authentik.tls.certresolver=letsencrypt"
  - "traefik.http.services.authentik.loadbalancer.server.port=9000"
```

### 2. Enable MFA

1. Navigate to **Flows & Stages** → **Stages** → **Create**
2. Create authenticator stages (TOTP, WebAuthn)
3. Add to authentication flows
4. Require for admin accounts at minimum

### 3. Firewall Rules

Only expose Authentik via reverse proxy:
```bash
# Allow only reverse proxy to access Authentik
iptables -A INPUT -p tcp --dport 9000 -s 172.18.0.0/16 -j ACCEPT
iptables -A INPUT -p tcp --dport 9000 -j DROP
```

### 4. Regular Backups

Backup PostgreSQL database regularly:
```bash
# Backup script
docker-compose exec -T postgresql pg_dump -U authentik authentik > backup-$(date +%Y%m%d).sql

# Restore
docker-compose exec -T postgresql psql -U authentik authentik < backup-20231115.sql
```

## Troubleshooting

### Can't Access Initial Setup Page

Check if Authentik is running:
```bash
docker-compose ps
docker-compose logs server
```

The initial setup is only available once. If you missed it:
```bash
# Reset and recreate admin user
docker-compose exec server ak create_admin_group
docker-compose exec server ak create_recovery_key
```

### OIDC Login Fails

1. Check Authentik logs: `docker-compose logs -f`
2. Verify redirect URI exactly matches in Tailscale
3. Ensure Authentik is accessible via HTTPS with valid certificate
4. Check issuer URL ends with trailing slash

### Database Connection Errors

```bash
# Check PostgreSQL is healthy
docker-compose ps postgresql

# Check credentials match in .env
grep PG_ .env

# Restart services
docker-compose restart
```

### Performance Issues

Authentik requires:
- **Minimum**: 1 CPU, 2GB RAM
- **Recommended**: 2 CPU, 4GB RAM for 100+ users
- **Database**: SSD recommended for PostgreSQL

## Integration with Other Services

### Grafana

In Grafana configuration:
```ini
[auth.generic_oauth]
enabled = true
name = Authentik
client_id = <client_id>
client_secret = <client_secret>
scopes = openid profile email
auth_url = https://auth.example.com/application/o/authorize/
token_url = https://auth.example.com/application/o/token/
api_url = https://auth.example.com/application/o/userinfo/
```

### Portainer

1. Navigate to **Settings** → **Authentication**
2. Select **OAuth**
3. Configure with Authentik endpoints
4. Users automatically created on first login

### Consul

Consul Enterprise supports OIDC for UI access. In Consul config:
```hcl
ui_config {
  enabled = true
}

acl {
  enabled = true
  default_policy = "deny"

  tokens {
    initial_management = "bootstrap-token"
  }
}
```

Then configure OIDC binding rules via Consul API.

## Maintenance

### Updating Authentik

```bash
# Edit .env to change version
nano .env  # Update AUTHENTIK_TAG=2024.8.4

# Pull new images
docker-compose pull

# Restart services
docker-compose up -d
```

Authentik handles database migrations automatically.

### Monitoring

Key metrics to monitor:
- PostgreSQL disk usage
- Redis memory usage
- HTTP response times
- Failed login attempts

Integrate with Prometheus:
```yaml
# Add to docker-compose.yml
- "traefik.http.routers.authentik-metrics.rule=Host(`auth.example.com`) && Path(`/metrics`)"
```

## Advanced: Multi-Tenancy

Authentik supports multiple **tenants** for different organizations:

1. Navigate to **System** → **Tenants** → **Create**
2. Each tenant gets its own:
   - Branding (logo, colors)
   - Domain
   - Default flows

Useful if you're hosting auth for multiple projects or environments.

## Resources

- [Authentik Documentation](https://goauthentik.io/docs/)
- [Tailscale OIDC Documentation](https://tailscale.com/kb/1240/sso-custom-oidc/)
- [dynipupdate README](../../README.md)

## Support

If you encounter issues:
1. Check Authentik logs: `docker-compose logs -f`
2. Review [Authentik Discussions](https://github.com/goauthentik/authentik/discussions)
3. Check Tailscale OIDC requirements
4. Verify DNS resolution via dynipupdate

## Security Considerations

- **Use strong passwords** for PG_PASS and admin accounts
- **Enable MFA** for all administrative accounts
- **Regular backups** of PostgreSQL database
- **HTTPS only** - never expose Authentik over HTTP
- **Firewall rules** to restrict access to Docker host
- **Keep updated** - watch for Authentik security advisories
