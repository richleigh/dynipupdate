# YubiKey Setup and Backup Strategy

This guide shows how to use YubiKey (or other FIDO2/WebAuthn hardware keys) with Authentik while ensuring you never lock yourself out.

## The Golden Rule: **Buy Two Keys**

**Never rely on a single hardware key.** Here's why:

- Keys can be **lost** (dropped in a lake, left in a taxi)
- Keys can be **damaged** (washing machine, run over by car)
- Keys can **fail** (electronics fail, even Yubico devices)
- You might **forget it** (travel without it, leave at office)

**Solution:** Buy at least 2 YubiKeys:
- **Primary key** - Keep on your keychain (daily use)
- **Backup key** - Store securely at home (safe, locked drawer)

Optional 3rd key:
- **Travel backup** - Keep in a separate bag when traveling

## Recommended Setup

### Hardware Keys

**YubiKey 5 Series (Recommended):**
- **YubiKey 5 NFC** ($55) - Works with phones, computers, NFC readers
- **YubiKey 5C NFC** ($55) - USB-C version
- **YubiKey 5 Nano** ($50) - Tiny, stays in USB port (good for backup key in safe)

**Budget Option:**
- **YubiKey Security Key** ($25-30) - FIDO2/WebAuthn only (no TOTP/PIV)
- Still excellent for Authentik MFA

**Where to buy:** [yubico.com](https://www.yubico.com) or Amazon

### Multi-Factor Authentication Strategy

Set up **multiple backup methods** in this priority order:

```
Primary Authentication
└─> Username + Password (strong, unique, in password manager)
    └─> MFA Layer 1: YubiKey #1 (daily use)
        └─> MFA Layer 2: YubiKey #2 (backup, in safe)
            └─> MFA Layer 3: TOTP app (phone + backup codes)
                └─> Recovery: Recovery codes (printed, in safe)
```

This gives you 4 ways to authenticate if something fails.

## Setup Guide

### Phase 1: Prepare Recovery Mechanisms

**Before adding hardware keys**, set up recovery options first:

#### 1. Generate Recovery Codes

In Authentik:
1. Log in as admin
2. Go to your user profile → **Settings**
3. Navigate to **MFA Devices** → **Static Tokens**
4. Click **Create Static Tokens**
5. **Print these codes** and store in a safe place
6. These are one-time use codes for emergency access

**Print them, don't just save digitally** - if your computer is locked, digital copies won't help.

#### 2. Set Up TOTP (Authenticator App)

1. In Authentik, go to **MFA Devices** → **TOTP**
2. Scan QR code with authenticator app:
   - **Recommended:** Aegis (Android), Raivo (iOS) - both support encrypted backups
   - **Alternative:** Authy, Google Authenticator, 1Password
3. **Backup the TOTP secret:**
   - Save QR code screenshot in password manager
   - OR write down the secret key manually
   - This lets you restore TOTP if you lose your phone

#### 3. Test TOTP Works

1. Log out of Authentik
2. Log back in using username/password + TOTP code
3. Confirm it works before proceeding

### Phase 2: Add Hardware Keys

Now you can safely add YubiKeys knowing you have recovery methods:

#### 1. Configure Authentik for WebAuthn

As admin, configure WebAuthn:

1. Navigate to **Flows & Stages** → **Stages**
2. Find or create **authenticator-validation** stage
3. Ensure **WebAuthn** is enabled in device types
4. Configure WebAuthn settings:
   - **Relying Party Name**: Your organization (e.g., "Home Network")
   - **Relying Party ID**: Your domain (e.g., `auth.example.com`)
   - **User verification**: `preferred` (fingerprint/PIN optional)
   - **Authenticator attachment**: `cross-platform` (allows USB keys)
   - **Resident key**: `discouraged` (better compatibility)

5. Save changes

#### 2. Enroll Primary YubiKey

1. Insert your **first YubiKey** into USB port
2. Log into Authentik user settings
3. Go to **MFA Devices** → **WebAuthn**
4. Click **Enroll WebAuthn Device**
5. Name it: "YubiKey Primary (Keychain)"
6. Follow prompts:
   - Browser will ask for permission
   - Touch your YubiKey when it blinks
7. Confirm it's listed in your devices

#### 3. Enroll Backup YubiKey

**Immediately** repeat with your second key:

1. Insert **second YubiKey**
2. Go to **MFA Devices** → **WebAuthn**
3. Click **Enroll WebAuthn Device**
4. Name it: "YubiKey Backup (Home Safe)"
5. Touch key when prompted

You should now see **2 WebAuthn devices** listed.

#### 4. Test Both Keys

**Critical: Test before storing backup key away!**

1. Log out of Authentik
2. Log in with username/password
3. When prompted for MFA, insert **primary key** and touch it
4. Confirm login works

Then:

1. Log out again
2. Log in with username/password
3. This time use **backup key** instead
4. Confirm it works

**Only after confirming both keys work**, store the backup key in your safe.

### Phase 3: Configure MFA Policy

Set up which MFA methods are required:

#### For Admin Accounts (You)

**Require WebAuthn OR TOTP:**

1. Navigate to **Policies** → **Create**
2. Type: **Authentication Policy**
3. Configure:
   - **Name**: "Admin MFA Required"
   - **Required MFA**: `2` (two factors total)
   - **WebAuthn**: Allowed
   - **TOTP**: Allowed (backup method)
   - **Static**: Allowed (recovery codes)

4. Bind to authentication flow

This means:
- Login requires password + (YubiKey OR TOTP)
- If you forget YubiKey, use TOTP from phone
- If phone is dead, use recovery codes

#### For Regular Users

Less strict for non-admin users:

- **Option 1:** Require MFA but allow TOTP only (no hardware key needed)
- **Option 2:** Make MFA optional but encouraged
- **Option 3:** Require hardware keys for sensitive groups only

### Phase 4: Secure Storage

**Primary Key:**
- Keep on keychain with house keys
- Use daily for all logins
- Replace immediately if lost

**Backup Key:**
- Store in fireproof safe or safety deposit box
- Test quarterly (mark calendar: "Test backup YubiKey")
- Never carry with primary key (defeats redundancy)

**Recovery Codes:**
- Print on paper (not just digital)
- Store in same safe as backup key
- Keep one set at trusted friend/family member's house (optional)

**TOTP Backup:**
- Encrypted backup of authenticator app (Aegis/Raivo support this)
- QR code screenshot in password manager
- Manual secret key written down

### Phase 5: Test Recovery Scenarios

**Test each scenario annually:**

#### Scenario 1: Lost Primary Key
1. Log in with backup key from safe
2. Remove lost key from Authentik
3. Order new primary key
4. Enroll new key when it arrives

#### Scenario 2: Lost Both Keys
1. Use TOTP from authenticator app
2. Log in successfully
3. Order replacement keys
4. Enroll new keys

#### Scenario 3: Lost Keys + Phone
1. Use printed recovery codes
2. Log in with recovery code
3. Re-enroll TOTP on new phone
4. Order new YubiKeys

#### Scenario 4: Total Disaster Recovery
If you lose everything (keys, phone, recovery codes):

**Prevention is key** - store one recovery code set at trusted location:
- Family member's house
- Safety deposit box
- Trusted friend

**Last resort** (if admin account):
- Access Authentik database directly (PostgreSQL)
- Reset MFA requirements via database
- **This is why you backup the database regularly**

## Integration with Services

Once YubiKeys are enrolled in Authentik, they work with all OIDC/SAML services:

### Tailscale (via Authentik OIDC)

1. User visits Tailscale login
2. Redirected to Authentik
3. Enter username/password
4. Touch YubiKey to complete login
5. Tailscale session established

**Key benefit:** One YubiKey protects access to your entire mesh network.

### Other Services (SSH, Web Apps, etc.)

YubiKey can also protect:

**SSH Access:**
```bash
# Generate SSH key on YubiKey (requires PIV support)
ykman piv keys generate --algorithm ECCP256 9a pubkey.pem
ykman piv certificates generate --subject "SSH Key" 9a pubkey.pem

# Use for SSH
ssh -I /usr/lib/x86_64-linux-gnu/opensc-pkcs11.so user@host
```

**PAM (Linux Login):**
```bash
# Install YubiKey PAM module
sudo apt install libpam-u2f

# Configure for sudo
echo "auth required pam_u2f.so" | sudo tee -a /etc/pam.d/sudo
```

**Web Apps:**
- Many apps support WebAuthn directly (GitHub, Google, Cloudflare)
- Or use Authentik SSO with YubiKey enforcement

## Advanced: YubiKey Features Beyond WebAuthn

YubiKeys support multiple protocols - you can use one key for many purposes:

### FIDO2/WebAuthn (Slot 1)
- **What we're using** for Authentik
- Unlimited accounts
- No configuration needed

### TOTP (Requires YubiKey 5)
- Store TOTP secrets on the key itself
- Use Yubico Authenticator app to view codes
- Backup if you lose phone

### PIV (Smart Card)
- Store certificates for SSH, Windows login, VPN
- Requires PIN
- Good for high-security environments

### OTP (Slots 2-3)
- Classic Yubico OTP (one-time passwords)
- Works with older systems
- Can use for password manager 2FA

### GPG/PGP
- Store GPG private keys on YubiKey
- Sign git commits, encrypt emails
- Key never leaves hardware

**Recommendation:** Start with WebAuthn only, add other features later if needed.

## Security Best Practices

### DO

✅ **Buy 2+ keys** from the start
✅ **Test backup key** before storing it away
✅ **Print recovery codes** on paper
✅ **Quarterly testing** of all recovery methods
✅ **Store backup key separately** from primary
✅ **Use strong passwords** (hardware key is 2nd factor, not replacement)
✅ **Keep firmware updated** (use YubiKey Manager)
✅ **Register keys to your name** at yubico.com (helps if found)

### DON'T

❌ **Don't rely on single key** (will lock yourself out eventually)
❌ **Don't skip recovery codes** (hardware fails)
❌ **Don't store backup with primary** (defeats redundancy)
❌ **Don't share keys** (each person gets their own)
❌ **Don't use same key for work and personal** (separation of concerns)
❌ **Don't ignore firmware updates** (security patches matter)
❌ **Don't forget to test** (untested backups don't work)

## Troubleshooting

### YubiKey Not Detected

**Browser compatibility:**
- **Chrome/Edge**: Full WebAuthn support ✅
- **Firefox**: Full support ✅
- **Safari**: WebAuthn support (macOS 14+, iOS 16+) ✅
- **Mobile browsers**: Check for NFC support

**USB issues:**
- Try different USB port
- Check USB-A vs USB-C adapter
- Ensure YubiKey LED blinks when inserted
- Try on different computer to rule out hardware failure

### Can't Enroll YubiKey

1. **Check Authentik configuration:**
   ```bash
   # Ensure WebAuthn is enabled
   docker-compose logs server | grep -i webauthn
   ```

2. **Verify domain configuration:**
   - Relying Party ID must match your domain
   - Must use HTTPS (WebAuthn requires secure context)
   - `localhost` works for testing only

3. **Browser console errors:**
   - Open DevTools (F12)
   - Check Console for WebAuthn errors
   - Common issue: Mixed content (HTTP + HTTPS)

### YubiKey Works But Not Recognized

**Reset the key (if needed):**
```bash
# Install YubiKey Manager
sudo apt install yubikey-manager

# Check YubiKey info
ykman info

# Reset FIDO2 (WARNING: Deletes all WebAuthn credentials!)
ykman fido reset
```

**Only do this if:**
- Key is malfunctioning
- You have backup key enrolled
- You're willing to re-enroll

### Touch Not Working

- **Ensure you touch the gold contact** (not the plastic)
- **Hold for ~1 second** (not just tap)
- **Try different angle** (flat, not sideways)
- **Check if LED blinks** (indicates waiting for touch)

### Locked Out: Recovery Procedure

If you lose access:

1. **Try TOTP** from authenticator app
2. **Try recovery codes** (printed sheet)
3. **Try backup YubiKey** (from safe)
4. **Database recovery** (admin access to PostgreSQL):
   ```bash
   # Connect to database
   docker-compose exec postgresql psql -U authentik

   # Disable MFA for user (emergency only)
   UPDATE authentik_core_user SET mfa_enabled = false WHERE username = 'your-username';
   ```

5. **Contact admin** (if not admin yourself)

## Cost Analysis

### Initial Setup

| Item | Cost | Notes |
|------|------|-------|
| YubiKey 5 NFC (Primary) | $55 | Daily use |
| YubiKey 5 NFC (Backup) | $55 | Store in safe |
| **Total** | **$110** | One-time |

**Budget option:** 2x Security Key ($50 total)

### Compared to Alternatives

| Solution | Cost | Security |
|----------|------|----------|
| SMS 2FA | Free | ⚠️ Vulnerable to SIM swap |
| TOTP App Only | Free | ✅ Good (but lose if phone lost) |
| Hardware Key (no backup) | $55 | ⚠️ Lockout risk |
| **2x YubiKeys + TOTP + Codes** | **$110** | ✅✅ **Excellent + Redundant** |
| Titan Security Keys (Google) | $60 | ✅ Good alternative |
| Duo Security (per user/month) | $3-9 | ✅ Good but recurring cost |

**$110 one-time investment** gives you top-tier security with proper redundancy.

## Lifecycle Management

### Annual Checklist

**Every year:**
- [ ] Test primary YubiKey login
- [ ] Test backup YubiKey login (retrieve from safe)
- [ ] Test TOTP app login
- [ ] Verify recovery codes are readable/accessible
- [ ] Check YubiKey firmware version (update if needed)
- [ ] Review and rotate passwords
- [ ] Review list of enrolled devices in Authentik
- [ ] Remove devices you no longer own

### Key Replacement Schedule

YubiKeys are very durable, but plan for replacement:

- **Expected lifespan:** 5-10 years
- **Firmware updates:** YubiKey 5+ get updates
- **Physical wear:** Replace if:
  - USB connector loose/damaged
  - Doesn't detect reliably
  - Physical cracks/damage
  - Water damage (unless waterproof model)

### Lost Key Procedure

**If you lose primary key:**

1. **Immediately log in** with backup key or TOTP
2. **Remove lost key** from Authentik:
   - Go to **MFA Devices** → **WebAuthn**
   - Find lost key
   - Click **Delete**
3. **Order replacement** from yubico.com
4. **When new key arrives:**
   - Enroll as new primary
   - Test it works
   - Update documentation

**Don't panic:** You have multiple backup methods.

## Group/Family Deployments

### Multiple Users

Each person needs their own keys:

**Family of 4:**
- Person 1: 2x YubiKeys
- Person 2: 2x YubiKeys
- Person 3: 2x YubiKeys
- Person 4: 2x YubiKeys
- **Total: 8 keys**

**Shared services backup:**
- Keep 1 "emergency admin" YubiKey in family safe
- Enrolled to shared admin account
- Only used if primary admin locked out

### Bulk Purchase

Yubico offers **volume discounts:**
- 10+ keys: ~10% off
- 50+ keys: ~20% off
- Useful for small businesses/families

## Integration with dynipupdate Stack

### Protecting Critical Services

**Authentik (OIDC Provider):**
- ✅ YubiKey required for admin login
- ✅ Regular users can use TOTP
- ✅ Recovery codes available

**Tailscale:**
- ✅ Protected via Authentik OIDC
- ✅ YubiKey required for network access
- ✅ Blocks unauthorized devices

**Consul:**
- ⚠️ No direct WebAuthn support
- ✅ Can use Authentik as OIDC provider (Consul Enterprise)
- ✅ Or protect with network ACLs (Tailscale)

**SSH Access:**
- ✅ Can require YubiKey for SSH (PIV)
- ✅ Or require Tailscale (which requires YubiKey)
- ✅ Defense in depth

### Recommended Policy

**Tier 1 (Highest Security) - YubiKey Required:**
- Authentik admin accounts
- Root/sudo access to servers
- Cloud provider accounts (AWS, DO, etc.)
- Domain registrar (CloudFlare)
- Financial accounts

**Tier 2 (High Security) - YubiKey or TOTP:**
- Tailscale access
- VPN access
- Email accounts
- Password manager
- Code repositories (GitHub)

**Tier 3 (Standard Security) - Password + Optional 2FA:**
- Media servers (Plex, Jellyfin)
- Internal web apps
- Development environments
- Monitoring dashboards

## Resources

### Official Documentation
- [Yubico WebAuthn Guide](https://developers.yubico.com/WebAuthn/)
- [Authentik MFA Documentation](https://goauthentik.io/docs/flow/stages/authenticator/)
- [FIDO Alliance Resources](https://fidoalliance.org/)

### Recommended Reading
- [Security Keys: Practical Cryptographic Second Factors for the Modern Web](https://www.usenix.org/conference/usenixsecurity17/technical-sessions/presentation/lang)
- [Google's Security Key Study](https://security.googleblog.com/2019/05/new-research-how-effective-is-basic.html)

### Tools
- [YubiKey Manager](https://www.yubico.com/support/download/yubikey-manager/) - Desktop app
- [Yubico Authenticator](https://www.yubico.com/products/yubico-authenticator/) - TOTP app
- [WebAuthn.io](https://webauthn.io/) - Test your YubiKey

### Purchase
- [yubico.com](https://www.yubico.com/store/) - Official store
- Amazon - Usually in stock
- Apple Store - Carries YubiKey 5Ci (USB-C + Lightning)

## Summary: The Safe Setup

**To never lock yourself out while staying secure:**

1. ✅ **Buy 2 YubiKeys** (primary + backup)
2. ✅ **Set up TOTP** as backup method
3. ✅ **Generate recovery codes** and print them
4. ✅ **Test all methods** before relying on them
5. ✅ **Store backup key separately** (safe at home)
6. ✅ **Print recovery codes** (store with backup key)
7. ✅ **Quarterly testing** (calendar reminder)
8. ✅ **Strong passwords** (hardware key is 2nd factor, not replacement)

**Result:** You have 4 independent ways to authenticate:
- Primary YubiKey (daily use)
- Backup YubiKey (in safe)
- TOTP app (on phone, backed up)
- Recovery codes (printed, in safe)

**Security + Redundancy = Peace of Mind** 🔐
