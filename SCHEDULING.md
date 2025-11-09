# Scheduling Dynamic DNS Updates

This guide covers how to set up automatic DNS updates on macOS and Linux.

## Prerequisites

1. **Docker installed and running**
2. **Configuration file created** at `~/.config/dynipupdate.env`:
   ```bash
   CF_API_TOKEN=your_token_here
   CF_ZONE_ID=your_zone_id
   HOSTNAME=your_hostname
   INTERNAL_DOMAIN=internal.example.com
   EXTERNAL_DOMAIN=external.example.com
   IPV6_DOMAIN=ipv6.example.com
   ```

3. **Test it works manually first**:
   ```bash
   docker run --rm --network host --env-file ~/.config/dynipupdate.env richleigh/dynipupdate:latest
   ```

---

## macOS Setup (launchd)

macOS uses `launchd` for scheduling tasks instead of cron.

### 1. Create the config directory

```bash
mkdir -p ~/.config
mkdir -p ~/Library/Logs
```

### 2. Copy and customize the plist file

```bash
# Copy the template
cp examples/scheduling/com.user.dynipupdate.plist ~/Library/LaunchAgents/

# Edit it to replace YOUR_USERNAME with your actual username
sed -i '' "s/YOUR_USERNAME/$USER/g" ~/Library/LaunchAgents/com.user.dynipupdate.plist

# Or edit manually
nano ~/Library/LaunchAgents/com.user.dynipupdate.plist
```

**Important customizations:**
- Replace `YOUR_USERNAME` with your actual macOS username
- Adjust the Docker image name if you're using a different registry
- Modify `StartInterval` (in seconds) to change frequency:
  - `300` = 5 minutes
  - `600` = 10 minutes
  - `3600` = 1 hour

### 3. Load the launch agent

```bash
# Load the agent
launchctl load ~/Library/LaunchAgents/com.user.dynipupdate.plist

# Verify it's loaded
launchctl list | grep dynipupdate
```

### 4. Monitor logs

```bash
# Watch the logs in real-time
tail -f ~/Library/Logs/dynipupdate.log

# Check for errors
tail -f ~/Library/Logs/dynipupdate.err.log
```

### 5. Management commands

```bash
# Stop the service
launchctl unload ~/Library/LaunchAgents/com.user.dynipupdate.plist

# Start the service
launchctl load ~/Library/LaunchAgents/com.user.dynipupdate.plist

# Run immediately (without waiting for interval)
launchctl start com.user.dynipupdate

# Check status
launchctl list | grep dynipupdate
```

### Troubleshooting macOS

**Service not running:**
```bash
# Check for errors in system log
log show --predicate 'subsystem == "com.apple.launchd"' --last 1h | grep dynipupdate

# Verify the plist is valid
plutil -lint ~/Library/LaunchAgents/com.user.dynipupdate.plist
```

**Permission issues:**
```bash
# Ensure the plist has correct permissions
chmod 644 ~/Library/LaunchAgents/com.user.dynipupdate.plist
```

---

## Linux Setup

Linux offers three options: cron, systemd timers, or systemd service with timer.

### Option 1: Cron (Simplest)

Works on all Linux distributions.

#### 1. Create log directory

```bash
mkdir -p ~/.local/log
```

#### 2. Edit crontab

```bash
crontab -e
```

#### 3. Add the cron job

```cron
# Run every 5 minutes
*/5 * * * * /usr/bin/docker run --rm --network host --env-file $HOME/.config/dynipupdate.env richleigh/dynipupdate:latest >> $HOME/.local/log/dynipupdate.log 2>&1
```

#### 4. Verify it's installed

```bash
crontab -l
```

#### 5. Monitor logs

```bash
tail -f ~/.local/log/dynipupdate.log
```

### Option 2: Systemd Timer (Modern Linux)

Systemd timers are more powerful and provide better logging integration.

#### 1. Install the service and timer files

```bash
# Create systemd user directory
mkdir -p ~/.config/systemd/user/

# Copy the files (replace 'rich' with your username)
cp examples/scheduling/systemd-timer/dynipupdate.service ~/.config/systemd/user/dynipupdate@.service
cp examples/scheduling/systemd-timer/dynipupdate.timer ~/.config/systemd/user/dynipupdate@.timer
```

#### 2. Enable and start the timer

```bash
# Reload systemd to recognize new files
systemctl --user daemon-reload

# Enable the timer to start on boot
systemctl --user enable dynipupdate@$USER.timer

# Start the timer now
systemctl --user start dynipupdate@$USER.timer
```

#### 3. Verify it's running

```bash
# Check timer status
systemctl --user status dynipupdate@$USER.timer

# List all timers
systemctl --user list-timers

# Check service status
systemctl --user status dynipupdate@$USER.service
```

#### 4. Monitor logs

```bash
# Watch logs in real-time
journalctl --user -u dynipupdate@$USER.service -f

# View recent logs
journalctl --user -u dynipupdate@$USER.service -n 50
```

#### 5. Management commands

```bash
# Stop the timer
systemctl --user stop dynipupdate@$USER.timer

# Disable the timer (won't start on boot)
systemctl --user disable dynipupdate@$USER.timer

# Run the service immediately (without waiting for timer)
systemctl --user start dynipupdate@$USER.service

# Restart the timer
systemctl --user restart dynipupdate@$USER.timer
```

### Troubleshooting Linux

**Cron not running:**
```bash
# Check if cron daemon is running
systemctl status cron    # Debian/Ubuntu
systemctl status crond   # RHEL/CentOS

# Check cron logs
grep CRON /var/log/syslog           # Debian/Ubuntu
journalctl -u crond                 # RHEL/CentOS with systemd
```

**Systemd timer not running:**
```bash
# Check timer status
systemctl --user status dynipupdate@$USER.timer

# Check for errors
journalctl --user -xe

# Enable lingering (allows user services to run without login)
loginctl enable-linger $USER
```

**Docker permission issues:**
```bash
# Add your user to docker group
sudo usermod -aG docker $USER

# Log out and back in for changes to take effect
```

---

## Schedule Recommendations

Choose a schedule based on your needs:

| Frequency | Use Case | Cron Format | Systemd OnCalendar |
|-----------|----------|-------------|--------------------|
| 5 minutes | Fast-changing IPs (mobile, coffee shops) | `*/5 * * * *` | `*:0/5` |
| 15 minutes | Home broadband with occasional changes | `*/15 * * * *` | `*:0/15` |
| 1 hour | Stable connection, belt-and-suspenders | `0 * * * *` | `hourly` |
| Daily | Very stable IP, just checking | `0 3 * * *` | `daily` |

**Recommendation:** Start with 5-10 minutes. CloudFlare's API is fast and has generous rate limits.

---

## Monitoring

### Check if DNS is updating

```bash
# Check your DNS records
dig anubis.e.4.bees.wtf
dig anubis.i.4.bees.wtf
dig anubis.6.bees.wtf AAAA
```

### Set up notifications (optional)

You can wrap the Docker command to send notifications on failure:

**macOS (with terminal-notifier):**
```bash
brew install terminal-notifier

# In your plist or cron:
docker run ... || terminal-notifier -title "DNS Update Failed" -message "Check logs"
```

**Linux (with notify-send):**
```bash
# In your cron or systemd service:
docker run ... || notify-send "DNS Update Failed" "Check logs"
```

---

## Uninstalling

### macOS

```bash
# Unload the service
launchctl unload ~/Library/LaunchAgents/com.user.dynipupdate.plist

# Remove the plist
rm ~/Library/LaunchAgents/com.user.dynipupdate.plist
```

### Linux (Cron)

```bash
# Edit crontab
crontab -e

# Remove the dynipupdate line, save and exit
```

### Linux (Systemd)

```bash
# Stop and disable the timer
systemctl --user stop dynipupdate@$USER.timer
systemctl --user disable dynipupdate@$USER.timer

# Remove the files
rm ~/.config/systemd/user/dynipupdate@.service
rm ~/.config/systemd/user/dynipupdate@.timer

# Reload systemd
systemctl --user daemon-reload
```

---

## Security Notes

1. **Protect your env file:**
   ```bash
   chmod 600 ~/.config/dynipupdate.env
   ```

2. **Use API tokens, not Global API Keys** - tokens can be scoped to specific permissions

3. **Monitor your logs** for authentication failures or unexpected behavior

4. **Keep the Docker image updated:**
   ```bash
   docker pull richleigh/dynipupdate:latest
   ```
