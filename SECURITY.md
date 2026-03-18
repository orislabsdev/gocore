# Security Policy

## Supported Versions

Only the latest major version of `gocore` is supported with security updates.

| Version | Supported          |
| ------- | ------------------ |
| v0.1.x  | :white_check_mark: |
| < v0.1  | :x:                |

## Reporting a Vulnerability

We take the security of `gocore` seriously. If you believe you have found a security vulnerability, please report it to us by emailing security@orislabs.dev.

Please do **not** open a public GitHub issue for security vulnerabilities.

After your report is received, we will:
1. Acknowledge receipt within 48 hours.
2. Investigate and confirm the vulnerability.
3. Provide an estimated timeline for a fix.
4. Notify you once the fix is released.

## Security Design Principles

`gocore` is built with a "security-first" mindset:
- **Default Secure**: All routes are private by default.
- **Minimal Dependencies**: We only use 2 external dependencies to reduce the attack surface.
- **Standard Library First**: We wrap `net/http` to leverage its battle-tested security.
- **Automated Protections**: Built-in middleware for HSTS, CSP, and rate limiting.

## Known Limitations

- **JWT Revocation**: `gocore` does not natively manage a JWT revocation list (blacklist). Revocation must be handled at the application layer or by using short-lived tokens and refresh token rotation.
- **In-Memory Rate Limiting**: The default rate limiter is in-memory and per-instance. For distributed systems, an external state (like Redis) should be used.
