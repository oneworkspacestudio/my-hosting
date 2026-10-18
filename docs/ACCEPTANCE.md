# Preview acceptance record — 18 October 2026

All checks used disposable accounts and isolated protected NTFS data folders.
The owner's existing MyAccounts, Drive and private app data were not fixtures.
Builds, dependency caches and public release files used the external work drive.
No GitHub Actions were run.

| Check | Evidence/result |
|---|---|
| Full native Go suite | Passed192.843s, pinned official Caddy2.11.7/Node24.22.0, actual Windows processes. |
| Next standalone | Next16.4.2 + React19.3.1 built84.28s and served server-rendered output through Caddy. |
| Nuxt Nitro | Nuxt4.6.2 + Nitro2.13.4 built40.90s using documented in-process Oxc and served through Caddy. |
| Static publication | ZIP upload, production switch, preview route, promotion/rollback, exact deployment-header probes passed. |
| Node isolation | Private host/cross-project reads and immutable output writes denied; own persistent directory write allowed. |
| Descendant cancellation | Real restricted parent + Node child stopped; completed cancellation waits for process exit; prior live traffic unchanged. |
| WebSocket and crashes | Actual upgrade + masked text frame/echo through Caddy; forced Node exit restarted with working traffic. |
| Caddy control | Unauthenticated access rejected; mTLS validated atomic reload/revision probes passed. No OS trust changes. |
| Runtime repair | Tampered runtime rejected; bad archive preserved old bytes; valid pinned archive repaired runtime. |
| Retention | Explicit cleanup retained live production/active previews/rollback dependencies/app data/backups; rejected paths outside allowlist. |
| Canonical domain routing | Native Caddy308 preserved path/query/deployment header; preview/local unaffected; HTTPS forwarded scheme; unverified canonical request rejected. |
| Retry/environment | Original encrypted snapshot retained by retry; rebuild uses current settings; late cancelled preparation not queued. |
| Webhooks | HMAC, wrong repo/SHA/branch, replay/deduplication and viewer-action denial checks passed. |
| OIDC/auth | Discovery/RSA JWT, PKCE/nonce, issuer/audience/expiry/email validation, replay, role/CSRF, logout checks passed. |
| Backup/recovery | Authenticated encrypted stream rejects corruption/truncation; new-folder restore preserved sources and portable secrets, stripped sessions and disabled public routes. |
| Fresh suite install | Actual Install path with selected Accounts + Hosting; protected paths, client registration, health and Caddy-only first static publish passed. |
| Real MyAccounts browser flow | One password sign-in opened Hosting. Workspace logout ended provider session and revoked Hosting database session (remaining0). |
| Workspace regressions | Full Go packages passed with NTFS test root, including update/recovery/catalog/navigation. |
| Public free tunnel | Actual production and browser-uploaded preview served valid public HTTPS and matching deployment headers. Existing DNS/ingress preserved. |
| UI | Real first-run/project wizard, light/dark/mobile checks, integrated sidebar and screenshots reviewed. |

Public synthetic examples on the owner's test zone:

- [Static production](https://hosting-demo-26-sites.m8e.co.uk/)
- [Static branch-style preview](https://deploy-129e44ffb0--hosting-demo-26-sites.m8e.co.uk/)

These are demonstrators, not a hosting service for other installations. Other
installations use their own connected domain and server. Preview URLs remain
active until intentionally retired; provider DNS cleanup is not automatic.

## Failures found and repaired during acceptance

- Metrics nested a settings query inside a one-connection SQLite row iteration;
  dashboard loading could hang. Settings are now fetched before iterating rows.
- Restricted Windows named pipes prevented framework compiler child stdio.
  Worker-thread/in-process framework configurations preserve the restrictions.
- Process-job active count could reach0 before descendant teardown completed.
  Cancellation now waits captured process handles before reporting completion.
- Provider cross-origin logout POST was rejected. Workspace now uses supported
  OIDC GET logout and explicitly revokes the path-scoped Hosting session.
- Empty local sites domain became invalid `sites.`; local-only installation now
  leaves it empty. Connected sites derive from the saved root DNS zone.
- Parallel Vault/Hosting provider writes could overwrite one another's route
  merges. Both now hold the same account/tunnel mutex across read/merge/write.

## Preview limits and acceptance not performed

Trusted administrators/project sources only. Restricted tokens and Job Objects
are defense in depth; this is not a VM or hostile multi-tenant service. One build
at a time, no outbound network sandbox, no managed serverless functions, no GitHub
App installation UX, no automatic preview/DNS expiry or namespace migration.
Framework/compiler configurations are version-specific; piped child stdio may
fail with EPERM. Custom domains need their own working DNS/route/certificate.

SCM services, unattended pre-login boot, physical machine reboot, Windows Server
Core, a second Windows machine and a cross-machine recovery drill have not been
accepted. Node22 is offered as a pinned offline/download runtime but the main
framework acceptance uses Node24. Binaries are unsigned if no signing certificate
is provided. Do not describe this preview as a production/stable service.
