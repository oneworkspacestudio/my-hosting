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

## Later corrections and successful checks

Working repository is I:/Projects/my-hosting. First explicitly unfinished source
checkpoint36d469f pushed to original org repo; Actions disabled and no tags/releases.
Restricted worker failure fixed: set project SID on restricted token object's DACL
and its default object DACL, create a private low-integrity desktop, launch suspended
with explicit inherited handles, assign job before resume. Negative own-write,
host/cross-project read/write and ACL-change tests pass. Full third Go run passed
validation/archive/redaction/encrypted framing/role/CSRF/queue/restart/audit tests.
Diagnostic restricted-token variants remain in policy_diagnostics_test.go; replace
with product acceptance tests rather than counting diagnostics as feature coverage.

Implemented atomic DB publication/revert routing, upload-staging recovery, frozen
portable environment export, retained source archives, base directories/detection,
Next static asset copying, persistent desired process stop, audit/access UI and
one-use local first-run setup capability. Later fixes need another compile/test.
12 real browser checks passed standalone owner setup/login, persisted project
wizard, navigation/API, dark theme/mobile overflow. UI assets281.91KB JS. Screenshots
docs/screenshots and external ui-first-acceptance.json contain synthetic state only.
Suite codex/my-hosting integration now uncommitted; targeted suite/distribution/
branding tests passed. Remaining lock/build-org/README/default installer pin not done.

Current Caddy implementation disables admin and uses validated --watch file reload.
Planned production improvement NOT implemented: owner-protected Windows AF_UNIX
admin socket with no TCP admin, supported atomic /load API, negative worker-connect
test, and cold-stop Caddy state during encrypted backup. Verify socket ACLs/path
limits and reload rollback before replacing existing implementation. Do not claim
runtime, Node/Next/Nuxt, full restore, real OIDC, service/reboot or installer checks.

## Approved storage cleanup and latest owner direction

Owner explicitly requested deletion of experimental backups (no archive), then
leave live Workspace on C: for now. No disk migration/repair/format/VHD authorized.
Both E/I are dirty exFAT. Removed all16 live-install rollback folders and old
installation copy, stopped/deleted this task's synthetic fixture and old Vault
fixture after path/reparse/process/service/task checks. Recovery list set[] under
update lock. Additional obsolete maintenance backup copies/source ZIP removed.
Recovered about22.8GiB; C: now23.20GiB free. Live owner suite preserved and healthy
locally/publicly. Do not delete personal Downloads, global caches or active runtime
compressed assets. Newest-backup retention was overridden only for this cleanup.
MyHosting test listener19007 is stopped; recreate fresh protected synthetic fixture
for remaining acceptance. Screenshots/non-secret receipts already on I. Continue
using I for large public build artifacts and bounded C NTFS fixtures with cleanup.

## Sites namespace, native/public acceptance and current preview

Owner resumed implementation and requested sites.m8e.co.uk. Explained nested
Cloudflare certificate coverage; scoped stored API token cannot read SSL packs
(403), and no sites/wildcard record existed. Owner briefly chose direct HTTPS,
then explicitly changed to the easier FREE tunnel option. Final agreed URLs:
`my-site-sites.m8e.co.uk` and `deploy-ID--my-site-sites.m8e.co.uk`. Other installs
use their own zone. No router edits or paid features; unused router tab closed.

Implemented nested/flat address styles and direct/tunnel transport, startup
defaults from Workspace, exact per-host proxied CNAME and tunnel ingress creation,
account/zone boundary/conflict checks and original-host preservation. No zone-wide
wildcard route. Cloudflare credentials are passed only to the control process,
never inherited by restricted builds. Suite launch integration uses protected
cloudflare-resources and HostingToken (MailDNSToken fallback). Core tests verify
DNS conflict refusal and preservation of other tunnel routes/settings.

IMPORTANT correction to previous socket plan: official Caddy2.11.7 Unix socket
startup FAILS on Windows because its listener unconditionally chmods socket files.
Abandoned this and removed AF_UNIX control code. Now plaintext admin disabled;
authenticated remote admin binds an ephemeral LOOPBACK TCP port, with mandatory
client certificate and only POST/load permission. Client key DPAPI/NTFS protected.
Dedicated internal CA/server identity, normal TLS verification and pinned CA,
install_trust=false: no OS/browser certificate-store changes. Caddy atomic load
and exact revision probe pass native switches/rollback. Backup stops Caddy+Node
and supervisor respects exclusive backup gate. Config failed-switch rollback
loads previous config through API. No --watch and no custom Caddy binary needed.

Windows process fixes: pass nil cwd for empty exec.Dir, explicit Caddy cwd, and
grant project SID READ_ATTRIBUTES/TRAVERSE/SYNCHRONIZE only on data/project path
ancestors because Node realpath stats each ancestor. No listing/content/write or
inherited grant. Host/cross-project reads and published-code writes remain denied.
Removed diagnostic-only policy test. Full native Go suite passed21.8seconds with
actual Caddy/Node archives, including static publish/previews/switch/rollback,
encrypted backup/restore of secrets+sources, Node publish/restart/isolation and
anonymous mTLS denial. UI build passed283.07KB. Later metrics/UI/public DNS helper
changes need rerun. Suite targeted Hosting/seamless tests pass. Logs on I:
backend-native-tests.log, publication-tests.log, node-tests*.log, suite-sites-tests.log.

Public synthetic preview CURRENTLY RUNNING, not installed in owner's catalog:
- root C:/Projects/.cache/myhosting-acceptance-20261018/public-preview (protected);
- binary I:/Projects/tooling/my-hosting-work/bin/MyHosting-new.exe, PID126808;
- management http://127.0.0.1:19007; Caddy loopback19080;
- synthetic test owner, disposable credentials only in external test scripts;
- project7a95f1361be78983cfe39b3a78b7dd43, slug hosting-demo-26;
- published retry852317335cec29569ffbf7568cb88dc0;
- public https://hosting-demo-26-sites.m8e.co.uk, exact owned DNS/tunnel rule
  added to existing Cloudflare connection. Other routes preserved.
Original deployment failed because Windows/router still resolved old wildcard
198.23... while Cloudflare/Google public DNS already had the correct proxy IPs.
Added fresh Cloudflare DoH, bounded IP-pinned HTTPS with normal hostname/CA checks
and progress logs. Retry published; standard Windows DNS subsequently refreshed,
Invoke-WebRequest200/header matched and Chrome DOM showed the actual page.

External scripts/receipts under tooling/my-hosting-work: start-public-acceptance.ps1
(-Resume for existing synthetic root), retry-public-acceptance.ps1, public-preview-
process.json and public-acceptance.json. First exe replacement failed on exFAT
(rename then Access Denied despite no live process); did NOT force ACLs. Running
new build at MyHosting-new.exe instead. Old public binary remains; source intact.
Stop/check only this owned preview after task verification; remove owned test
DNS/ingress or migrate the sample deliberately before deleting its data, avoiding
stranded502 routes. Do not stop owner's live Workspace/tunnel or test its accounts.

CUA bindings: hostingTab IAB10 authenticated synthetic dashboard, marked deliverable;
publicChrome Chrome585733629 successfully loaded public site, screenshot attempt
timed out before markDeliverable. IAB11 stuck on internal network-error data URL
from old DNS; getTab hit URL policy (do not inspect/bypass that data page).
Repo screenshot docs/screenshots/publication-live.png saved via CUA and viewed.
Public site's HTTP+TLS+Chrome proof succeeded. Current screenshots are narrow
responsive IAB width, not desktop marketing mockups. Fix discovered public traffic
counter matching (old prefix excluded flat hosts), added exact host set and
mobile nav aria-labels; not yet rebuilt/rerun after these latest edits.

Remaining original gates still require work; README/docs/IMPLEMENTATION explicitly
unfinished. New docs/PUBLISHING.md describes free URL setup, TLS and data boundaries.
App first GitHub source checkpoint still36d469f; subsequent changes not pushed yet.
Suite feature branch still uncommitted; organization release pin/build-org/README,
brand exports and paired publication unfinished. Continue actual SSR/OIDC/installer
acceptance; do not replace required checks with current public static success.

## Follow-up acceptance in progress

Source now includes OIDC fixture provider tests covering RSA JWT discovery/code,
PKCE/nonce, explicit owner vs viewer identity, unverified email, invalid issuer/
audience/expiry, callback replay and logout. All passed in full native suite39.9s
(backend-oidc-tests.log). Flat-host metrics matching and mobile nav accessible
labels fixed; latest frontend build283.48KB. Added publication settings to /api/me
and suggested URL in project wizard. Added read-only runtime ancestor metadata
grants for npm's own main module and a real restricted static build check; rerun
native tests after these last edits. Caddy/Node/other-process isolation still
passes in latest completed run. Original six Caddy mTLS settings reloads work.

Brand exports generated from original SVG using existing bundled Sharp (no C
dependency installation): PNG64/128/256/512 and ICO, branding/export.cjs. Suite
overview regenerated with balanced6columns/17icons at app-suite-368eccf3.png;
README links that hash and adds My Hosting row marked integration in progress.
App README links actual publication-live.png; docs explain final free URL pattern.

Public Chrome tab585733629 DOM confirmed the actual successful site. Screenshot
timed out, so no public-site.png was created and deliverable marking after that
call did not run. IAB10 still marked deliverable with local dashboard. Live Caddy
PID126008 at19080 belongs only to synthetic preview; manager126808 uses NEW exe.
Normal system DNS eventually resolved Cloudflare IPs and ordinary HTTPS request
200/header matched. Do not claim IAB11 succeeded (old internal error page).

Added real framework fixtures under examples/next and examples/nuxt: Next16.4.2,
React19.3.1, Nuxt4.6.2 from official npm metadata. Next package-lock generated.
Nuxt `npm install --package-lock-only --ignore-scripts --audit=false --fund=false`
is STILL RUNNING in exec session95900, no lock yet at last check. Cache/TEMP on I.
Native SSR test15887 also RUNNING with MYHOSTING_SSR=1 and actual Caddy/NodeZIPs;
hosting.test PID122024, root unit-3634010272, Next npm child117840. It is installing
Next dependencies through the restricted worker; log deploybb8f0b03065b1baae0b7fac39da01c2e.
Monitor log/npm cache, not live owner data. Next then Nuxt run sequentially; Nuxt
requires package-lock to finish. Reports ssr-tests.log; no SSR success claimed yet.
Default tests skip heavy SSR unless explicitly enabled. C free22.74GiB at last check.

No commits beyond initial app36d469f yet; both worktrees have intended changes.
Commit/push an explicitly unfinished source checkpoint after tests/log checks to
preserve work on warning-state I. Paired release/install gates still incomplete.


## Late 18 October corrective handoff (supersedes stale resume above)

Latest pushed source checkpoint is e4827e0, not36d469f. The old running Nuxt lock/
SSR sessions finished. Native Next16.4.2 and Nuxt4.6.2 both build and serve via
Caddy; Nuxt uses pinned in-process Oxc instead of esbuild's denied named pipes.
Full final Go suite passed192.843s (Next84.28/Nuxt40.90); verified archives under
external tooling/my-hosting-work/downloads. WebSocket frame exchange and Node
crash recovery passed11.06s. Cancellation now captures and waits descendant
process handles, because job active-count0 can precede process-object signaling;
strict immediate descendant-exit and unchanged-live-site checks pass8.24s.
No escaped test process remained from the initial failed timing assertion.

Implemented metrics single-connection deadlock fix, preview ZIP selection and
republish guard, immutable retry env snapshots, preparation/cancellation/queue
serialization, safe explicit retention, canonical redirects/HTTPS forwarding,
verified staged runtime repair and a shared user/SYSTEM Cloudflare named mutex.
The identical mutex in Workspace preserves simultaneous Vault/Hosting route
updates. Framework runtime artifacts discard build-only dependencies. Sidebar
return link no longer overlaps the suite switcher. Settings/operations explicitly
state SCM service/other-machine/reboot/ServerCore acceptance is unavailable.

Fresh synthetic suite install through actual Install helper passed with managed
Accounts and Hosting, Caddy downloaded on first static deploy and no Node needed.
Latest disposable root C:/Projects/.cache/myhosting-suite-acceptance-20261018,
manager PID117584 (verify current receipt), localhost19300, provider19301,
Hosting19317. New helper scripts/native-hosting-acceptance.go isolates APPDATA.
Browser single sign-in opens Hosting; provider GET logout and remembered
path-scoped app token fix passed actual browser flow; remaining Hosting sessions0.
Full suite tests pass with GOTMPDIR on protected C: (Go1.27 t.TempDir uses
GOTMPDIR, not just TEMP; external exFAT test failures were environment failures).
Stop/remove this exact fixture after final testing, never owner live data.

Public synthetic preview remains separate: localhost19007, Caddy19080; source
root public-preview below the app acceptance parent. Managed production
https://hosting-demo-26-sites.m8e.co.uk and browser-uploaded preview
https://deploy-129e44ffb0--hosting-demo-26-sites.m8e.co.uk both served valid HTTPS
and matching deployment headers. Existing tunnel/DNS preserved. Its process
receipt outside Git is authoritative; old PID/binary above is stale. Upgrade
preview deliberately and retain or remove exact owned public routes on cleanup.

Added operational/startup docs, actual API specification, compatibility manifest,
local packaging/notices/dependency inventory script and Express fixture. No
Actions (both org repositories disabled). No release yet at this handoff.
Next: finish packaging, real GitHub Express deployment, source commits, publish
accepted preview app asset, pin its verified digest in suite org lock/build-org,
full bundle installer/catalog checks then paired preview release. No stable or
SCM/other-machine claims. Preserve live C:/tmp/workspace and unrelated histories.


## Final real GitHub and restart follow-up

Pushed accepted source checkpoint a483058. Real GitHub Express fetch exposed
GitHub's leading PAX global revision comment; extractor now accepts bounded
non-path global metadata, still rejects traversal/overrides. Regression passes.
Actual process restart exposed individually protected runtime child ACLs ignoring
inheritable root grants. allowRuntime now reapplies per-project read/execute to
every safe child. Native reset/restore/npm test passes14.11s. Generic framework
restrictions remain unchanged; no write/secret permissions broadened.

GitHub Express project4a871cc0d54b2867d202732b158721cc successfully installed68
packages, built from pinned public commit a483058, and published deploy
e24fe9a75fc731e445243bbd8fce0b53 at https://hosting-node-26-sites.m8e.co.uk.
Earlier failed attempts retained; no failure claimed as success. Static demo
latest republish f06c451eea0c38b68927993db05ee73c also published, prior preview
remains active. Current public process receipt uses restart-candidate binary;
upgrade it to final release bytes and verify Node survives actual manager restart.

Local packaging script now generates attribution from compiled Go dependencies
and frontend runtime licenses, JSON dependency inventory, three official runtime
pins, compatibility and startup/docs/examples. Earlier script errors (unused
module missing dir, Go VCS stamping and npm empty-key lock) fixed. First ZIP was
only a packaging check and predates latest runtime fix; do not publish stale bytes.
Full release regression including SSR is running externally. Suite build-org and
catalog guard now require Hosting but lock still needs actual published digest.


Final full regression including both real SSR frameworks passed205.683s after
archive/restart fixes (Next82.10s, Nuxt47.36s, WebSocket/crash13.66s). Source
440bdee is pushed. Earlier packaging checks are not release bytes. Final source
commit adds this evidence only; local ZIP/build-info must point at that commit.
Still no published release at this recording; next is app draft/upload/verify/
publish, exact asset digest pin in suite and paired verified installer release.
