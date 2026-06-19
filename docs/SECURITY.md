# Security Policy and Procedures
<!-- LEGACY: audit marker for issue #61. -->

> WARNING: This security document is a LEGACY document. The security team
> maintains the authoritative security documentation in the internal security
> portal. This document is a simplified version intended for developers and
> may not reflect the most current security policies. Always check the security
> portal for the latest requirements before implementing security controls.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 3.x | ✅ Active support |
| 2.x | ⚠️ Security fixes only (EOL: 2024-06-30) |
| 1.x | ❌ No longer supported |

## Reporting a Vulnerability

Report vulnerabilities to security@example.com (not a real address).

We aim to acknowledge receipt within 24 hours and provide an initial assessment
within 72 hours. Critical vulnerabilities are triaged immediately.

### Encryption

Please encrypt sensitive vulnerability reports using our PGP key:

```
Key ID: 0xDEADBEEF
Fingerprint: ABCD 1234 EF56 7890 ABCD 1234 EF56 7890 ABCD 1234
```

The PGP key is published on the public keyserver network and on our security
portal. If you cannot verify the fingerprint, send the report unencrypted.

### Disclosure Policy

We follow a 90-day disclosure timeline for coordinated disclosure:

- Day 0: Report received and acknowledged
- Day 1-7: Triage and severity assessment
- Day 8-45: Fix development and testing
- Day 46-60: Fix deployment to production
- Day 61-90: Public disclosure coordination

We request that researchers allow us 90 days before public disclosure. We will
work with researchers to coordinate disclosure timing and credit.

## Security Practices

### Authentication

- All API endpoints require authentication except:
  - `/health` (health check)
  - `/auth/login` (login)
  - `/auth/register` (registration)
  - `/auth/reset-password` (password reset)
  - `/auth/verify-email` (email verification)
- Authentication uses JWT Bearer tokens with RS256 signatures
- Tokens expire after 60 minutes
- Refresh tokens expire after 30 days
- MFA is required for admin accounts and recommended for all users
- Rate limiting is applied to all authentication endpoints
- Account lockout after 5 failed login attempts

### Authorization

- Role-based access control (RBAC) with the following roles:
  - `admin`: Full system access
  - `trader`: Trading and account access
  - `analyst`: Read-only access to market data
  - `viewer`: Read-only access to public data
  - `api_only`: Programmatic access with restricted permissions
- Permission checks are performed at the API gateway level
- Sensitive operations require additional authorization checks

### Data Protection

- All data in transit is encrypted using TLS 1.3
- All data at rest is encrypted using AES-256
- Database encryption keys are managed by AWS KMS
- Secrets are stored in HashiCorp Vault
- PII is encrypted at the column level in the database
- Logs are sanitized to remove PII before storage
- Backups are encrypted before transfer to S3

### Input Validation

- All user input is validated server-side
- SQL injection is prevented through parameterized queries
- Cross-site scripting (XSS) is prevented through output encoding
- Cross-site request forgery (CSRF) is prevented through token validation
- File uploads are scanned for malware
- Maximum request size is limited to 10MB

### Rate Limiting

- General API: 100 requests/second per API key
- Authentication endpoints: 10 requests/second per IP
- Order placement: 50 requests/second per user
- WebSocket messages: 1000 messages/second per connection
- Rate limit violations result in HTTP 429 responses

### Audit Logging

- All authentication events are logged
- All configuration changes are logged
- All permission changes are logged
- All data access events are logged (for GDPR compliance)
- All financial transactions are logged
- Audit logs are immutable and append-only
- Audit logs are retained for 365 days

## Compliance

### Regulatory Compliance

- **GDPR**: We are GDPR compliant. Data subject access requests are handled
  within 30 days. Data retention policies are documented and enforced.
- **SOC 2**: We are SOC 2 Type II certified. Our most recent audit was
  completed in Q4 2023 with no material findings.
- **CCPA**: We are CCPA compliant for California residents.
- **MiFID II**: Our trading platform is MiFID II compliant for EU clients.
- **FINRA**: We comply with FINRA rules for US broker-dealer activities.

### Security Certifications

- SOC 2 Type II (audited annually)
- ISO 27001 (certified)
- PCI DSS Level 1 (for payment processing)
- CSA STAR Level 2

## Incident Response

### Response Plan

1. **Detection**: Incident detected by monitoring, alerting, or external report
2. **Triage**: Assess severity and impact (15 min SLA for SEV1)
3. **Containment**: Isolate affected systems (30 min SLA for SEV1)
4. **Eradication**: Remove root cause (4 hour SLA for SEV1)
5. **Recovery**: Restore normal operations (8 hour SLA for SEV1)
6. **Post-Mortem**: Document lessons learned (48 hour SLA)

### Contact Information

- Security team: security@example.com
- On-call engineer: Contact via PagerDuty
- Emergency: +1-555-0123 (not a real number)

## Dependency Management

### Automated Scanning

- All dependencies are scanned by Dependabot (GitHub)
- Critical and high severity vulnerabilities are flagged within 24 hours
- Automated PRs are created for fixable vulnerabilities
- Weekly full dependency audit reports are generated

### Update Policy

| Severity | Update Window | Approval |
|----------|---------------|----------|
| Critical | 48 hours | Security team lead |
| High | 7 days | Team lead |
| Medium | 30 days | Standard PR review |
| Low | Next release | Standard PR review |

### Pinning Policy

- Runtime dependencies are pinned to exact versions
- Development dependencies allow semver-compatible ranges
- All dependencies are reviewed before addition
- Deprecated dependencies are replaced within 90 days
