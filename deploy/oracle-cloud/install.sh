#!/bin/bash
set -e

echo "=== DynIP Update - Oracle Cloud Free Tier Setup ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (use sudo)"
    exit 1
fi

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Install required packages
echo "Installing dependencies..."
dnf install -y golang || apt-get install -y golang

# Create app directory
echo "Creating application directory..."
mkdir -p /opt/dynipupdate
mkdir -p /etc/dynipupdate

# Build the binaries
echo "Building binaries..."
cd "$SCRIPT_DIR/../.."
go build -o /opt/dynipupdate/dynipupdate .
go build -o /opt/dynipupdate/cleanup ./cmd/cleanup

# Copy configuration template
echo "Creating configuration file..."
cat > /etc/dynipupdate/config.env <<'EOF'
# CloudFlare API Configuration
CF_API_TOKEN=your-cloudflare-api-token-here
CF_ZONE_ID=your-cloudflare-zone-id-here

# DNS Domain Names (set exact names you want)
INTERNAL_DOMAIN=myhost.internal.example.com
EXTERNAL_DOMAIN=myhost.external.example.com
IPV6_DOMAIN=myhost.ipv6.example.com
COMBINED_DOMAIN=myhost.example.com

# Instance identifier (used in heartbeat)
INSTANCE_ID=oracle-vm-1

# Hostname (auto-detected if not set)
HOSTNAME=$(hostname)

# CloudFlare Proxy (true/false)
CF_PROXIED=false

# Cleanup Configuration (for cleanup service only)
STALE_THRESHOLD_SECONDS=3600    # 1 hour
CLEANUP_INTERVAL_SECONDS=300    # 5 minutes
EOF

chmod 600 /etc/dynipupdate/config.env
echo "Configuration template created at /etc/dynipupdate/config.env"
echo "Please edit this file with your CloudFlare credentials and domain names"

# Install systemd service for updater
echo "Installing updater service..."
cat > /etc/systemd/system/dynipupdate.service <<'EOF'
[Unit]
Description=Dynamic DNS Updater for CloudFlare
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
EnvironmentFile=/etc/dynipupdate/config.env
ExecStart=/opt/dynipupdate/dynipupdate
StandardOutput=journal
StandardError=journal
SyslogIdentifier=dynipupdate

[Install]
WantedBy=multi-user.target
EOF

# Install systemd timer for updater (runs every 5 minutes)
echo "Installing updater timer..."
cat > /etc/systemd/system/dynipupdate.timer <<'EOF'
[Unit]
Description=Run Dynamic DNS Updater every 5 minutes
After=network-online.target

[Timer]
OnBootSec=1min
OnUnitActiveSec=5min
AccuracySec=1s

[Install]
WantedBy=timers.target
EOF

# Install systemd service for cleanup (optional)
echo "Installing cleanup service..."
cat > /etc/systemd/system/dynipupdate-cleanup.service <<'EOF'
[Unit]
Description=DNS Record Cleanup Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/dynipupdate/config.env
ExecStart=/opt/dynipupdate/cleanup
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=dynipupdate-cleanup

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
echo "Reloading systemd..."
systemctl daemon-reload

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Edit configuration: sudo nano /etc/dynipupdate/config.env"
echo "2. Add your CloudFlare API token and zone ID"
echo "3. Set your domain names"
echo "4. Enable and start the updater:"
echo "   sudo systemctl enable --now dynipupdate.timer"
echo "5. (Optional) Enable cleanup service if this is your primary VM:"
echo "   sudo systemctl enable --now dynipupdate-cleanup.service"
echo ""
echo "Check status:"
echo "  sudo systemctl status dynipupdate.timer"
echo "  sudo journalctl -u dynipupdate -f"
echo ""
echo "Run updater manually:"
echo "  sudo systemctl start dynipupdate.service"
echo ""
