# My Hosting release and review handoff, 18 October 2026

## Actual publication

- Original Apache-2.0 repository, not a fork; upstream fit findings in
  docs/IMPLEMENTATION.md. No GitHub Actions; both repositories have Actions disabled.
- App release v0.1.0-preview.1, source a0375b832feb8acbadba22cfb71456501a67f1de:
  https://github.com/oneworkspacestudio/my-hosting/releases/tag/v0.1.0-preview.1
- App ZIP SHA256392273b43dc97c468e395ab0010276864b6ac70a719dcb2c18d7b09b81ebd129.
  Anonymous public download and GitHub digest verified. Includes exe, notices,
  dependency inventory, runtime pins, compatibility, examples, API/operations docs.
- Paired suite v0.3.0-preview.36, source33aa6dd4f74961d29483e9870398b22fa596b541:
  https://github.com/oneworkspacestudio/one-workspace/releases/tag/v0.3.0-preview.36
  All20 uploaded asset sizes/digests verified before publication. Public catalog15
  apps. All75 bundle members and embedded installer payload verified. Prior app
  executable pins unchanged. Both source branches pushed; suite main updated.

## Acceptance and fixes

Final native suite205.683s (Next82.10s/Nuxt47.36s), actual Caddy/Node, WebSocket
frames, crash restart, process-descendant cancellation, runtime permission reset,
retention, canonical redirects, encrypted new-folder recovery, OIDC/roles/CSRF.
Real GitHub Express installed68 packages and served public HTTPS from pinned
source. PAX global metadata and runtime descendant ACL fixes were caught by actual
integration, repaired and regression-tested. See earlier initial handoff for
architecture and failures. SCM/ServerCore/reboot/cross-machine tests not accepted;
trusted administrators/projects only, one build, documented compiler limitations.

Fresh final FULL suite bundle installed selected Accounts/Hosting. Real browser
single sign-in opened Hosting. ZIP upload provisioned verified Caddy and published
in9s; no Node installed for static. Logout left zero Hosting sessions. Program-only
manual removal preserved app DB and MyAccounts, then disposable suite removed.
No owner data used as fixtures. Actual updater contract download/extraction/hash
passed; public suite/app discovery recognizes36/v1 with no spurious update.

## Important publisher contract

Current optional installs retain four Hosting files: exe + three notices. Eight
supplemental guide/inventory packages in the raw bundle are not retained. Marker
must scope to actual installed four, with canonical9007 app metadata. A raw12-file
marker was correctly rejected; corrected published marker SHA256
422f4d12db717b89e9f06a740d6e713111ee5b20c0fd6a52d04dca6fc4a9c879.
Do not weaken validator or export a runtime test port. See PUBLISHING.md.

## Review services and cleanup

Disposable C:/Projects/.cache/myhosting-suite-acceptance-20261018 is stopped and
removed, including its isolated APPDATA shortcut. Unit fixtures removed. Owner
C:/tmp/workspace remains running/healthy; never stop or use its accounts as fixtures.
Only current public demo is retained for the requested review while keep/stop
preference is pending (question cites AGENTS cleanup rule). Do not treat this as
permission to leave obsolete test suites running. Stop it after review if not kept.

Current standalone demo receipt external: tooling/my-hosting-work/public-preview-
process.json. Program release-a0375b8/MyHosting.exe under that tooling root;
protected data C:/Projects/.cache/myhosting-acceptance-20261018/public-preview,
management127.0.0.1:19007, Caddy19080. Real released bytes restarted; all three public
sites recovered with same headers/URLs. No service/autostart registered for demo.
Exact owned hosts: hosting-demo-26-sites.m8e.co.uk, hosting-node-26-sites.m8e.co.uk,
deploy-129e44ffb0--hosting-demo-26-sites.m8e.co.uk. Before decommission, remove only
these matching DNS/ingress entries under shared account/tunnel mutex; preserve
owner's tunnel and unrelated routes. Never strand502 routes or delete owner data.

External receipts/builds: tooling/my-hosting-work/release-a0375b8 and
workspace-hosting-release36. Source/build caches on I:; private data needs NTFS.
I:/E: exFAT warnings remain; no repair/reformat authorized. Live suite stays C:.
Post-release commits are documentation/handoff only; tagged executable source
SHAs above remain authoritative. No further product work required for this preview.
