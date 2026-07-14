# Security Policy

## Supported Versions

ORAG is currently preparing its first public Beta. Security fixes are provided for the latest published prerelease on a best-effort basis. After `v0.1.0-beta.1`, the support table will be maintained as follows:

| Version | Supported |
| --- | --- |
| Latest prerelease | Yes |
| Older prereleases | No |
| Unreleased `main` | Development fixes only |

Experimental capabilities may change quickly, but security reports affecting them are still handled through this policy.

## Reporting a Vulnerability

Use GitHub Private Vulnerability Reporting:

1. Open the repository's **Security** tab.
2. Select **Advisories** and **Report a vulnerability**.
3. Create a private security advisory with the affected version, environment, reproduction steps, impact, and any suggested mitigation.

Direct link: [privately report a security vulnerability](https://github.com/shikanon/orag/security/advisories/new).

Do not open a public Issue, Discussion, or Pull Request for an undisclosed vulnerability. Do not include real credentials, personal data, proprietary documents, or production traces. Use synthetic evidence and redact secrets.

## Response Targets

These are response targets, not contractual service-level agreements:

- acknowledgement within 3 business days;
- initial severity and scope assessment within 7 business days;
- status updates at least every 7 business days while remediation is active;
- coordinated disclosure after a fix or mitigation is available.

Reports are prioritized using exploitability, affected deployments, data exposure, tenant-boundary impact, and availability risk. Maintainers may request additional reproduction details in the private advisory.

## Disclosure Process

When a report is accepted, maintainers will:

1. confirm affected versions and components;
2. prepare and privately validate a fix or mitigation;
3. assign a CVE through GitHub when appropriate;
4. publish patched artifacts and a security advisory;
5. credit the reporter if requested and safe to do so.

Please allow a reasonable remediation window before public disclosure. If a report is out of scope or not reproducible, maintainers will explain the decision in the private advisory.

## Security Boundaries

- Deterministic mock providers are test and demo tools, not production security boundaries.
- Default development credentials must be replaced before any shared or internet-accessible deployment.
- Agent self-operations must not bypass explicit approval, audit, idempotency, or rollback checks.
- Provider keys and object-storage credentials belong in secret injection mechanisms, never Git.
