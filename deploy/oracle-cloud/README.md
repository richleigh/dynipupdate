# Oracle Cloud Free Tier Deployment

This guide helps you run the Dynamic DNS Updater on Oracle Cloud's Always Free tier.

## Oracle Cloud Free Tier Specs

- **AMD VMs**: 2 VMs with 1/8 OCPU and 1 GB RAM each
- **ARM VMs**: Up to 4 Ampere A1 VMs (24 GB RAM total)
- **Cost**: $0/month forever

## Quick Setup

### 1. Create Oracle Cloud VM

1. Sign up for Oracle Cloud: https://www.oracle.com/cloud/free/
2. Create a new Compute instance (Ubuntu or Oracle Linux)
3. Choose "Always Free" shape (ARM Ampere A1.Flex or AMD)
4. Save your SSH key and note the public IP

### 2. Install on Oracle VM

SSH into your VM:
```bash
ssh ubuntu@your-vm-ip
```

Clone the repository:
```bash
git clone https://github.com/richleigh/dynipupdate.git
cd dynipupdate
```

Run the installation script:
```bash
sudo bash deploy/oracle-cloud/install.sh
```

### 3. Configure

Edit the configuration file:
```bash
sudo nano /etc/dynipupdate/config.env
```

Set your values:
```bash
# Get these from CloudFlare dashboard
CF_API_TOKEN=your-token-here
CF_ZONE_ID=your-zone-id-here

# Set exact full domain names you want created
INTERNAL_DOMAIN=oracle-vm.internal.example.com
EXTERNAL_DOMAIN=oracle-vm.external.example.com
IPV6_DOMAIN=oracle-vm.ipv6.example.com
COMBINED_DOMAIN=oracle-vm.example.com

# Identifier for this VM
INSTANCE_ID=oracle-free-tier-vm1
```

**Get CloudFlare API Token:**
1. Go to CloudFlare Dashboard → My Profile → API Tokens
2. Create Token → Edit Zone DNS template
3. Select your zone → Continue → Create Token
4. Copy the token

**Get Zone ID:**
1. CloudFlare Dashboard → Select your domain
2. Scroll down on Overview page → Zone ID on right sidebar

### 4. Start the Service

Enable and start the updater:
```bash
sudo systemctl enable --now dynipupdate.timer
```

**Optional** - Enable cleanup service (only on ONE VM):
```bash
sudo systemctl enable --now dynipupdate-cleanup.service
```

### 5. Verify It's Working

Check timer status:
```bash
sudo systemctl status dynipupdate.timer
```

View logs:
```bash
sudo journalctl -u dynipupdate -f
```

Run manually to test:
```bash
sudo systemctl start dynipupdate.service
```

## What Gets Created

The updater will create these DNS records in CloudFlare:

```
oracle-vm.internal.example.com  → 10.0.0.5 (private IP)
oracle-vm.example.com           → 132.145.xxx.xxx (Oracle public IP)
_heartbeat.oracle-vm.internal.example.com → "timestamp,oracle-free-tier-vm1"
```

## Cleanup Service

The cleanup service deletes stale DNS records (older than 1 hour).

**Run cleanup ONLY on one VM** to avoid conflicts.

Enable it:
```bash
sudo systemctl enable --now dynipupdate-cleanup.service
```

Check status:
```bash
sudo systemctl status dynipupdate-cleanup.service
sudo journalctl -u dynipupdate-cleanup -f
```

## Update Frequency

- **Updater**: Runs every 5 minutes (via systemd timer)
- **Cleanup**: Checks every 5 minutes, deletes records >1 hour old

## Multiple VMs

If you have multiple Oracle VMs, configure each with unique domains:

**VM 1:**
```bash
INTERNAL_DOMAIN=oracle-vm1.internal.example.com
EXTERNAL_DOMAIN=oracle-vm1.external.example.com
IPV6_DOMAIN=oracle-vm1.ipv6.example.com
COMBINED_DOMAIN=oracle-vm1.example.com
INSTANCE_ID=oracle-vm1
```

**VM 2:**
```bash
INTERNAL_DOMAIN=oracle-vm2.internal.example.com
EXTERNAL_DOMAIN=oracle-vm2.external.example.com
IPV6_DOMAIN=oracle-vm2.ipv6.example.com
COMBINED_DOMAIN=oracle-vm2.example.com
INSTANCE_ID=oracle-vm2
```

## Troubleshooting

### Check if service is running:
```bash
systemctl status dynipupdate.timer
systemctl list-timers dynipupdate.timer
```

### View logs:
```bash
journalctl -u dynipupdate --since "1 hour ago"
```

### Test CloudFlare API:
```bash
source /etc/dynipupdate/config.env
curl -X GET "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_API_TOKEN}" \
  -H "Content-Type: application/json"
```

### Run manually:
```bash
sudo systemctl start dynipupdate.service
sudo journalctl -u dynipupdate -n 50
```

### Check what IPs are detected:
```bash
# Internal IP
ip addr show | grep "inet "

# External IP
curl -4 https://api.ipify.org
curl -6 https://api6.ipify.org
```

## Firewall / Security Groups

Oracle Cloud VMs have restrictive firewalls by default. You may need to:

1. **Allow outbound HTTPS** (should be allowed by default)
2. No inbound ports needed (DNS updater is client-only)

## Uninstall

```bash
sudo systemctl stop dynipupdate.timer
sudo systemctl stop dynipupdate-cleanup.service
sudo systemctl disable dynipupdate.timer
sudo systemctl disable dynipupdate-cleanup.service
sudo rm /etc/systemd/system/dynipupdate.*
sudo rm /etc/systemd/system/dynipupdate-cleanup.service
sudo rm -rf /opt/dynipupdate
sudo rm -rf /etc/dynipupdate
sudo systemctl daemon-reload
```

## Cost

**Free!** Oracle Cloud Always Free tier includes:
- VMs run 24/7 at no cost
- Outbound network transfer (CloudFlare API calls) is free
- No charges as long as you stay within free tier limits

## Updates

To update to the latest version:

```bash
cd ~/dynipupdate
git pull
sudo bash deploy/oracle-cloud/install.sh
sudo systemctl restart dynipupdate.timer
sudo systemctl restart dynipupdate-cleanup.service  # if running
```
