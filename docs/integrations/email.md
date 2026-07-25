---
title: Email Integration
description: SMTP-based email notifications for alert delivery, investigation updates, and password resets — raw SMTP with PLAIN auth, multipart HTML, and 3x retry.
---

# Email Integration

Alga sends email notifications via raw SMTP (`net/smtp`) with optional TLS and PLAIN authentication. Emails are dispatched through the `EmailWorker` on the `alga.email.send` RabbitMQ queue with 3x retry. When `SMTP_HOST` is empty, email delivery is log-only (messages are logged but not sent).

## Configuration

### Environment Variables

Configure email delivery via these environment variables:

```sh
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=alga@example.com
SMTP_PASSWORD=your-smtp-password
SMTP_FROM=noreply@example.com
SMTP_SKIP_TLS_VERIFY=false
```

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SMTP_HOST` | | No | SMTP relay hostname. When empty, email delivery is log-only (messages are logged but not sent). |
| `SMTP_PORT` | `587` | No | SMTP port (typically 587 for STARTTLS, 465 for SMTPS) |
| `SMTP_USER` | | No | SMTP authentication username (PLAIN auth). Only needed when the relay requires authentication. |
| `SMTP_PASSWORD` | | No | SMTP authentication password (PLAIN auth). Only needed when the relay requires authentication. |
| `SMTP_FROM` | `alga@localhost` | No | From address for email notifications |
| `SMTP_SKIP_TLS_VERIFY` | `false` | No | When `true`, skips TLS certificate verification for SMTP (implicit-TLS handshake with `InsecureSkipVerify`). Enabling this is an MITM risk and logs a loud warning on startup. |

**Security Notes:**
- Treat `SMTP_PASSWORD` as sensitive data — store in `.env` file or secure secrets management
- Never commit credentials to version control
- Use environment-specific credentials in production
- Consider using application-specific passwords or API keys

See [Configuration](/configuration/environment-variables) for complete environment variable reference.

## Email Content

Alga constructs email content as plain strings via the notification dispatcher. There are no Go template files; the dispatcher builds subject and body text directly from the alert, investigation, or incident data.

### Example Output Format

#### Alert Notification Email

**Subject:** `[critical] HighMemoryUsage — firing`

**Body:**
```
Alert: HighMemoryUsage
Status: firing
Severity: critical
Fingerprint: abc123def456

View in Alga: https://alga.example.com/alerts/abc123def456
```

#### Investigation Update Email

**Subject:** `Investigation Update: HighMemoryUsage`

**Body:**
```
Investigation: HighMemoryUsage
Status: investigating
Agent: sre-agent-1

View in Alga: https://alga.example.com/investigations/42
```

## Notification Preferences

Users can configure email delivery via their notification preferences in the Alga UI.

See [Notification Preferences](/on-call/notification-preferences) for complete preference documentation.

### Setting Email Preferences

1. Navigate to **Profile** → **Notification Preferences**
2. Configure email delivery rules:
   - Enable/disable email notifications
   - Set severity filters (info, warning, critical)
   - Configure quiet hours
   - Set default notification channel

### Notification Types

| Notification Type | Description |
|-------------------|-------------|
| `alert_created` | New alert received |
| `alert_acknowledged` | Alert acknowledged |
| `alert_resolved` | Alert resolved |
| `investigation_created` | New investigation |
| `investigation_updated` | Investigation update |
| `incident_created` | New incident |
| `incident_acknowledged` | Incident acknowledged |
| `incident_resolved` | Incident resolved |
| `mention` | User mentioned in comment |
| `escalation` | Escalation triggered |

## Email vs Plain Text

Alga supports both plain text and HTML email formats.

### Multipart Messages

Alga sends multipart emails by default:

```
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alga-boundary

--alga-boundary
Content-Type: text/plain; charset=utf-8

Plain text version of message...

--alga-boundary
Content-Type: text/html; charset=utf-8

<html>HTML version of message...</html>

--alga-boundary--
```

Email clients display the HTML version if supported, falling back to plain text.

## Rate Limiting

### SMTP Provider Limits

Different SMTP providers have varying rate limits:

| Provider | Rate Limit | Quota Period |
|----------|------------|--------------|
| Gmail | 100 emails/day | 24 hours |
| SendGrid | 100 emails/day (free tier) | 24 hours |
| Mailgun | 5,000 emails/month | 30 days |
| Amazon SES | 200 emails/day (sandbox) | 24 hours |
| Postmark | 100 emails/minute | 1 minute |
| Office 365 | 10,000 recipients/day | 24 hours |

### Alga Rate Limiting

Alga processes email notifications through the `EmailWorker` on the `alga.email.send` RabbitMQ queue. Failed deliveries are retried up to 3 times before being dead-lettered.

### Best Practices

1. **Batch notifications:** Group similar alerts into digest emails
2. **Deduplicate:** Use notification preferences to avoid duplicate emails
3. **Quiet hours:** Configure off-hours to reduce overnight volume
4. **Severity filtering:** Only email critical/warning alerts by default
5. **Upgrade plans:** Use paid SMTP tiers for higher limits in production

## Bounce Handling

### Email Delivery Status

Alga tracks email delivery attempts in the `notification_delivery_logs` table. Each delivery record includes the channel, status, and any error details.

### Retry Logic

Email failures are retried up to **3 times** by the `EmailWorker` (`alga.email.send` queue). After exhausting retries, the message is dead-lettered.

### Bounce Classification

Common bounce scenarios:

| Bounce Type | SMTP Code | Description | Action |
|-------------|-----------|-------------|--------|
| Hard bounce | 5xx | Permanent failure (invalid address) | Review user email address |
| Soft bounce | 4xx | Temporary failure (mailbox full) | Retried up to 3x, then dead-lettered |
| Rate limit | 421 / 452 | Too many emails | Reduce send rate |
| Timeout | - | SMTP server timeout | Retried up to 3x, then dead-lettered |

### Monitoring Bounces

Monitor email delivery issues via:

```bash
# Review logs for email delivery errors
grep "email" /var/log/alga/app.log
```

## Testing Email Delivery

### Send Test Email

Trigger a test email via notification preferences:

1. Navigate to **Profile** → **Notification Preferences**
2. Click **Send Test Notification**
3. Check your inbox for test email

### Manual Test via API

Create a test alert to trigger email delivery:

```bash
curl -X POST http://localhost:8080/api/v1/alerts \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SESSION_TOKEN" \
  -d '{
    "fingerprint": "test-email-alert",
    "labels": {
      "alertname": "Test Alert",
      "severity": "critical"
    },
    "annotations": {
      "summary": "Testing email delivery",
      "description": "This is a test alert to verify email notifications"
    },
    "status": "firing"
  }'
```

### Debug Mode

Enable debug logging for email operations:

```sh
LOG_LEVEL=debug
```

Review logs for detailed email delivery information:
```
[INFO] email sent to user@example.com: [critical] Test Alert — firing
[DEBUG] notification dispatch: email channel handled for user 123
```

## Troubleshooting

### SMTP Connection Failures

**Issue:** `dial tcp: lookup smtp.example.com: no such host`

**Solution:**
- Verify `SMTP_HOST` is correct and DNS resolves
- Check network connectivity to SMTP server
- Review firewall rules allow outbound SMTP traffic
- Test connectivity: `telnet smtp.example.com 587`

**Issue:** `x509: certificate signed by unknown authority`

**Solution:**
- Ensure SMTP server has valid SSL/TLS certificate
- Fix certificate chain on SMTP server

### Authentication Failures

**Issue:** `535 Incorrect authentication data`

**Solution:**
- Verify `SMTP_USER` and `SMTP_PASSWORD` are correct
- Check if password requires special encoding (URL-encoded, base64)
- For Gmail, use app-specific password instead of account password
- Some providers require "From" address to match authenticated username

**Issue:** `530 Must issue a STARTTLS command first`

**Solution:**
- Verify `SMTP_PORT` is set to 587 for STARTTLS
- Some providers require explicit TLS upgrade
- Check SMTP server supports STARTTLS

### Email Not Received

**Issue:** Email sent successfully but not received

**Solution:**
1. Check email delivery logs in Alga
2. Verify user's notification preferences enable email
3. Check spam/junk folder
4. Verify SPF/DKIM records for sending domain
5. Check SMTP provider delivery logs
6. Ensure recipient email address is valid

**Issue:** "SMTP not configured" error in logs

**Solution:**
- Verify all SMTP environment variables are set
- Check `.env` file is loaded by Alga
- Restart Alga after changing environment variables
- Verify no typos in variable names

### Rate Limiting Issues

**Issue:** `421 Too many messages from this IP`

**Solution:**
- Reduce email worker prefetch count
- Implement batch notifications instead of per-alert emails
- Upgrade SMTP provider plan for higher limits
- Configure quiet hours to reduce overnight volume

## Security Considerations

### TLS Configuration

- **Always use TLS** in production
- **STARTTLS (port 587)** is preferred over legacy SMTPS (port 465)
- Verify certificate chain is valid and complete
- Use strong TLS versions (TLS 1.2+)

### Credential Management

- Store SMTP credentials in environment `variables` or secrets management
- Never commit credentials to version control
- Use application-specific passwords when available (Gmail, Office 365)
- Rotate SMTP credentials regularly (quarterly)
- Implement access logging for SMTP configuration changes

### Content Security

- Sanitize user-generated content in email bodies
- Use template escaping to prevent email injection attacks
- Limit email size to avoid DoS via large payloads
- Validate recipient email addresses format
- Implement rate limiting per user to prevent abuse

### SPF/DKIM/DMARC

Configure DNS records for your sending domain:

**SPF (Sender Policy Framework):**
```
example.com. IN TXT "v=spf1 include:_spf.google.com ~all"
```

**DKIM (DomainKeys Identified Mail):**
- Generate DKIM keys via your SMTP provider
- Add public key to DNS as TXT record
- Sign all outbound emails

**DMARC (Domain-based Message Authentication):**
```
_dmarc.example.com. IN TXT "v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com"
```

## Common SMTP Providers

### Gmail

```sh
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=app-specific-password
SMTP_FROM=your-email@gmail.com
```

**Setup Instructions:**
1. Enable 2FA on Google Account
2. Generate App-Specific Password: Google Account → Security → App passwords
3. Use app-specific password as `SMTP_PASSWORD`

### SendGrid

```sh
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=SG.YOUR_API_KEY
SMTP_FROM=noreply@example.com
```

### Mailgun

```sh
SMTP_HOST=smtp.mailgun.org
SMTP_PORT=587
SMTP_USER=postmaster@mg.example.com
SMTP_PASSWORD=your-mailgun-password
SMTP_FROM=noreply@example.com
```

### Amazon SES

```sh
SMTP_HOST=email-smtp.us-east-1.amazonaws.com
SMTP_PORT=587
SMTP_USER=AKIAIOSFODNN7EXAMPLE
SMTP_PASSWORD=BLongStringWith+Special/Characters=
SMTP_FROM=noreply@example.com
```

**Setup Instructions:**
1. Verify sending domain in AWS SES Console
2. Create SMTP credentials via SES → SMTP Settings
3. Request production access (out of sandbox)

### Microsoft 365 / Exchange Online

```sh
SMTP_HOST=smtp.office365.com
SMTP_PORT=587
SMTP_USER=your-email@example.onmicrosoft.com
SMTP_PASSWORD=your-password
SMTP_FROM=your-email@example.onmicrosoft.com
```

## API Reference

### Integration Endpoints

```bash
# Get integration status
GET /api/v1/integrations

# Update integration settings
PUT /api/v1/integrations

# User notification preferences
GET /api/v1/users/me/notification-preferences
PUT /api/v1/users/me/notification-preferences
POST /api/v1/users/me/notification-preferences/test
```

## See Also

- [Configuration](/configuration/environment-variables) — Complete environment variable reference
- [Notification Preferences](/on-call/notification-preferences) — User notification settings
- [Integrations](/integrations/) — All integration documentation
- [Slack Integration](/integrations/slack) — Slack notification setup
- [Mattermost Integration](/integrations/mattermost) — Mattermost notification setup
