# Security policy

## Supported version

Only the current `main` branch and the production image revision referenced by the deployment environment receive security fixes. Historical commits and rollback images are supported only long enough to perform an emergency additive-schema-compatible rollback.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability or exposed credential.

Use GitHub's private vulnerability reporting for this repository:

1. Open the repository's **Security** tab.
2. Select **Advisories**.
3. Select **Report a vulnerability**.

Include the affected endpoint or component, reproduction steps, expected impact, and any suggested mitigation. Remove real credentials, personal data, raw production logs, and visitor message content from the report.

## Response priorities

- Exposed credential or active exploitation: revoke/contain immediately.
- Authentication, authorization, secret-boundary, RCE, injection, or data-loss issue: highest priority.
- Availability or abuse-control bypass: high priority when remotely exploitable.
- Dependency findings are triaged by reachable call path; a non-reachable module finding does not by itself authorize weakening CI.

## Disclosure

Please allow time for validation, remediation, release verification, and production rollout before public disclosure. Production changes follow the protected environment and safe change-window process.
