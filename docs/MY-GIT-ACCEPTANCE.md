# My Git source acceptance — 2 November 2026

All new functional checks use synthetic identities/data on isolated protected
NTFS roots. Large sources/caches/artifacts use the external work drive. No owner
projects, credentials or private repositories were test fixtures.

- Full app Go suite and Workspace Go packages passed.
- Chrome production wizard passed repository name, browser URL, .git URL,
  detected non-main branch, private-source creation and early ambiguous-name
  error at desktop/phone widths. Screenshots reviewed; no physical-phone claim.
- Native Gitea28.1.0 fixture: private repo metadata/contents/archive and immutable
  commit passed. Admin impersonation denied another user's private repo;
  disabled and unlinked users denied. No email-based account linking.
- Actual Hosting/Caddy static site and npm/Node24 build from that private repo
  passed, returning the expected site over HTTP with immutable source proof.
- Signed My Git and GitHub hook authentication and durable deduplication passed.
- SSRF/private-address/redirect, immutable ID, actor binding, credential scope,
  legacy GitHub project, worker environment and missing manager checks passed.

No data schema migration is introduced. Existing project JSON without a provider
continues using GitHub. All new connection authority is assigned server-side.
My Git requires Workspace110; standalone GitHub/ZIP paths remain available.
Existing trusted-project/unsigned-Windows-preview, service-mode, reboot and
cross-machine recovery limitations remain. Public endpoint/installer delivery
receipts are recorded separately; these development checks do not claim them.


Preview3 also accepts a static directory containing one descriptive .html/.htm
file without index.html. It copies that page to index.html in the isolated build
output; the source repository/archive remains unchanged. An existing index is
preserved; ambiguous, hidden-only and linked entry pages are rejected.
