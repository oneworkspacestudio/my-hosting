# Installation, operation and recovery

## Windows prerequisites

Windows 10/11 AMD64 and a local NTFS data folder with persistent ACLs are required.
Use a normal Windows account, never LocalSystem. The server refuses builds when
its account is SYSTEM or restrictions cannot be applied. Windows Server desktop
installations may work, but Server Core, SCM service accounts and reboot behavior
have not been accepted on a separate machine in this preview.

`MyHosting.exe` embeds its dashboard and SQLite. Caddy and, when required, Node
are separately managed processes. Closing the dashboard tab does not stop them.
Do not place private data on exFAT, a network share or a synced cloud folder.

## One Workspace

Select **My Hosting** in the installer. The manager creates its own data folder,
registers an OIDC client, passes the explicit owner subject and launches it at
`/hosting`. An existing Workspace login opens Hosting without another password.
Workspace sign-out revokes that browser's Hosting session and navigates through
the provider's supported OIDC logout. It does not revoke unrelated devices.

In a connected Cloudflare installation, the existing scoped connection creates
each site's exact hostname and tunnel route. Publishing waits for actual public
HTTPS and the expected deployment revision. See [publishing](PUBLISHING.md).
Unchecking Hosting preserves the app's data; use the suite's normal app/update
management instead of manually replacing its catalog hashes.

## Standalone

Extract the ZIP, choose an NTFS folder, and use the included
`MyHosting.example.ps1` as a launch configuration. The default management address
is `http://127.0.0.1:9007`. The first browser launch receives a one-use setup link;
create the standalone administrator there. `setup` can reopen setup before an
owner exists. Configuring MyAccounts disables standalone bootstrap.

Flags precede the command. For an existing registered MyAccounts client, set
`WORKSPACE_OIDC_ISSUER`, `WORKSPACE_OIDC_CLIENT_ID`,
`WORKSPACE_OIDC_CLIENT_SECRET` and `WORKSPACE_OWNER_SUBJECT`; register the exact
`<origin>/auth/callback` URL. Never put the secret in a command-line argument or
source control. Site content must use another origin than the dashboard.

Management binds only to loopback. Publish it through an authenticated Workspace
installation or an explicitly configured reverse proxy; never expose Caddy's
control endpoint. Standalone provider tokens are not collected by the dashboard.
Workspace supplies its protected Cloudflare connection to the control process.

## Background startup and Windows services

The supported suite path is Workspace's **start when I sign in** option. It runs
under the same Windows account and supervises child apps. Standalone users can
register the included `scripts/Register-Startup.ps1` for startup at user logon.
Neither requires a password to be stored in a script. Windows must remain awake.

This preview is **not an SCM service executable**. Do not run `sc.exe create`
against it or run it as SYSTEM. Service wrappers, Session 0/private desktops,
unattended startup before login and Windows Server Core are not yet accepted.
Do not mistake a successful restart test for a machine reboot/service test.

## Runtimes and offline use

Settings can download and verify pinned official Caddy and Node ZIPs or import
the exact same ZIP offline. `runtime-pins.json` in the release lists URL/version/
SHA-256. Static prebuilt ZIPs require Caddy only. Builds and server apps provision
the selected Node major. No global Node or Git installation is needed to run
the app. npm builds still need their dependencies or a suitable offline source.

**Repair runtime & restart apps** stages and verifies replacement bytes before
replacing the old runtime. Affected apps pause; project files and secrets remain.
An unavailable or corrupt download is reported without replacing the old bytes.

## GitHub and commands

Use an administrator's fine-grained PAT restricted to the selected repositories
with read access to contents/metadata. A GitHub App installation flow is not
implemented. Save the PAT in project Git settings; it is DPAPI encrypted and
never returned to the UI or passed to workers. Configure the displayed webhook
URL and HMAC secret in GitHub, content type JSON, push events. Automatic builds
respect branch allowlists and deduplicate deliveries. Configure webhook retries
at the sender if the server is offline or maintenance rejects admission.

Commands are argument arrays using managed `node`/`npm`; shell command strings
are not accepted. Only trusted sources should be approved for executable builds.
See [tested framework configurations](FRAMEWORKS.md) for Windows compiler limits.

## Deployments and persistent files

Rebuild uses retained source with current settings/secrets. Retry keeps the
original immutable configuration, source revision and environment snapshot.
Republish reuses retained artifacts and their environment; it does not rebuild.
Preview republish stays a preview; **Promote** intentionally switches production.
Rollback switches to a retained healthy artifact and records a new deployment.

Write application data only in `MYHOSTING_DATA_DIR` (the project's `var` folder),
never in immutable release output. Sources, releases, logs, SQLite and encrypted
secrets have named directories below the data root. Cleanup is explicit and
preserves live production, active previews, retained dependencies, app data,
backups and audit metadata. Older unused deployment files can become expired.

## Backup and migration

Export an encrypted backup in Settings using a strong independent password.
The backup pauses managed traffic/processes for consistency. Keep its password
separately. Browser sessions are excluded; protected project secrets are portable
inside the encrypted backup. Never copy raw DPAPI blobs to another machine.

Stop the target app, then restore into a **new**, empty NTFS folder:

```powershell
Read-Host 'Backup password' -MaskInput | .\MyHosting.exe --backup 'E:\safe\hosting.mhbackup' --restore-to 'C:\HostingRestored' --password-stdin restore
```

Use PowerShell 7 for `-MaskInput`. Reconfigure the host origin, MyAccounts client
and callback; import/provision runtimes. Restores leave public routing disabled
until reviewed. Same-machine fresh-folder restore is tested; another-machine
DPAPI recovery, service startup and full OS reboot are not yet accepted.

## Updates and uninstall

Use Workspace updates for managed installations. For standalone, back up first,
stop the app and its managed children, verify the release checksum, replace the
program folder and restart with the same data folder. Never merge databases or
restore over a live root. SQLite schema migrations run at startup.

Unregister standalone startup with `Register-Startup.ps1 -Remove`, stop the app,
then remove only its program files. Keep its data folder by default. Before
decommissioning public projects, remove their exact owned DNS/tunnel routes;
this preview does not automatically garbage-collect provider DNS on uninstall.

## Troubleshooting

| Symptom | Action |
|---|---|
| NTFS/ACL error | Select a local NTFS data folder before starting; do not weaken ACL checks. |
| `spawn EPERM` in a framework build | Use the tested worker-thread/in-process compiler config or upload prebuilt output. |
| DNS/HTTPS check pending | Inspect deployment logs and exact DNS/tunnel route; allow local DNS TTL to refresh. Existing conflicts are preserved. |
| Port in use | Stop the unrelated listener or change the unused local port before publishing. External Caddy is never adopted. |
| Runtime checksum changed | Use Settings runtime repair with an official pinned archive. |
| Crash loop | Inspect process logs, fix the project, redeploy or explicitly restart after fixing persistent configuration. |
| Login repeats | Check exact origin/issuer/callback and the Workspace/provider session. Independent standalone cookies are not suite SSO. |

Changing an existing public namespace/transport is an administrator migration:
old provider routes are not silently rewritten or deleted. Use the same routing
settings for routine upgrades. Automated namespace migration is not implemented.
