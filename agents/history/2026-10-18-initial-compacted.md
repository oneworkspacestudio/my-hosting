# My Hosting implementation

Owner supplied the MyHosting master brief and authorized implementation,
integration, tests, commits/pushes and paired releases after checks pass.
No GitHub Actions remains a session requirement. This is an original app after
upstream fit/licensing assessment; see docs/IMPLEMENTATION.md. Do not claim a fork.
Real suite source is I:/Projects/one-workspace; new app source/public artifacts on
E: per owner's storage preference. E/I are exFAT. Private test state requires NTFS.
Initial C: free space about400MB, so no large C builds/fixtures. A dedicated new
VHDX test disk script was prepared on E:, but Windows elevation was cancelled;
no disk was created/formatted. Continue coding independently; obtain a usable
isolated NTFS test volume before substantive deployment/security acceptance.
Existing owner installation must not be used as a development fixture.

Dependencies under assessment: Go1.27.2, SQLite modernc1.60.1, OIDC3.21.0,
Caddy2.11.7 official Windows ZIP digest recorded in runtime lock when added.
UI uses AUI compiler0.2.1. No implementation/acceptance/release completion yet.

## Storage correction

Windows reports E: exFAT health Warning / Full Repair Needed. npm dependency
content on E was invalid. Stopped the task-owned npm process and rescued all39
UTF-8 source files to I:/Projects/my-hosting, verifying both destination and repeat
source SHA256 reads. Reinitialized the new app Git repository on I (no commits
existed on E). E source left untouched; do not resume E writes or run drive repair
without owner authorization. Reinstall dependencies from official sources into
I caches. Source rescue receipt: tooling/my-hosting-work/source-rescue.json.
Code is preliminary and backend compilation passed before rescue; no tests yet.

## First implementation checkpoint

Backend Go and AUI-generated React/Vite dashboard compile on I after fresh npm
installation. Unit tests passed project validation, unsafe ZIP/case duplicates,
split-write secret redaction, authenticated encrypted backup corruption/truncation,
role/CSRF/write-only secret handling and queued environment/restart recovery.
Restricted Windows worker starts suspended but exits DLL initialization0xc0000142;
adding the session logon SID did not fix it. Do not bypass restrictions or publish.
Next step is a dedicated low-integrity desktop and explicit CreateProcessAsUser
startup rather than Go exec default desktop. Retain negative host/cross-project
file/ACL tests. Larger runtime fixtures still need protected NTFS space.

I: also reports Warning / Full Repair Needed, although fresh dependency install
and compilation succeeded there. Preserve source with an explicitly unfinished
GitHub checkpoint; no binary release while critical acceptance remains incomplete.
C: has only about400MB free, so unit roots are tiny and cleaned after tests under
C:/Projects/.cache/myhosting-acceptance-20261018. Never use owner live data.

Outstanding code review fixes: atomic DB publication + revert route on DB error;
uploading-state recovery and failure cleanup; retention/canonical redirects;
Node process desired stop state; Next standalone static/public copying; versioned
portable backup must also re-encrypt deployment_env on another machine; complete
role/UI audit path and avoid duplicate process starts. Suite integration is not yet
implemented. Current UI generated/semantic adapter uses permissive inferred loop
types due compiler0.2.1; AUI is the authoritative screen source. No Actions.
