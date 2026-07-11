# Production deployment

GitHub Actions publishes `ghcr.io/panyakorn04/portfolio-backend-2026:<commit-sha>` and deploys only that immutable tag. The versioned script updates only `BACKEND_IMAGE` in `/opt/apps/.env`, preserving every other setting and the existing Compose overlays. It gates success on `https://api.panyakorn.com/api/studio/overview` and restores the prior image automatically on failure.

Manual rollback: run **CI/CD → Run workflow**, choose `rollback`, and enter a previously published full 40-character commit SHA. Deployments are serialized across the production environment.

Required GitHub environment (`production`) secrets: `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`, `VPS_KNOWN_HOSTS`. `VPS_KNOWN_HOSTS` must contain the pinned OpenSSH known_hosts line(s), not a fingerprint. The workflow uses its short-lived repository-scoped `GITHUB_TOKEN` with `packages: read` while deploying and removes the temporary Docker credential directory afterward. No application secrets are copied or changed.
