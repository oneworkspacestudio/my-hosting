<p align="center"><img src="branding/my-hosting.svg" alt="My Hosting" width="112" height="112"></p>
<h1 align="center">My Hosting</h1>
<p align="center">Your sites and apps. Your Windows server. One Workspace.</p>

**Implementation in progress — no supported release yet.** This original
One Workspace Studio application is being built as a native Windows hosting
control plane with a branded React dashboard, Go, SQLite and managed Caddy/Node
processes. It is not a fork of Netlify, Coolify, Dokploy or Caddy.

## Intended workflow

Connect a GitHub repository or upload a site ZIP, select a static/Node/Next/Nuxt
adapter, follow a deployment's real log, and promote or restore retained artifacts.
MyAccounts uses normal OIDC discovery, authorization code, PKCE and nonce.
Standalone bootstrap is disabled when MyAccounts is configured.

Source and current acceptance plan: [implementation decisions](docs/IMPLEMENTATION.md).
The dashboard is authored in [AUI source](web/src/screens) and compiled to React.
The original [logo source and provenance](branding/README.md) follow the suite's
indigo, mint and silver icon family.

## Current development status

Server/API and dashboard sources are implemented and compile. The initial unit
checks cover archive rejection, log redaction across write boundaries, encrypted
backup corruption/truncation, role/CSRF/secret visibility and job recovery.
Windows restricted-worker startup and full runtime acceptance are still being
worked through. Suite integration, complete operational acceptance and paired
releases are not yet complete. Do not install this source checkpoint as a
production service or expose untrusted builds.

## Security scope

The initial scope is **trusted administrators and trusted project sources**.
Project build scripts execute code. Windows low-integrity restricted tokens,
per-project ACLs and Job Objects are defense in depth, not a VM boundary or a
claim that untrusted multi-tenant hosting is safe. The implementation fails closed
if required restrictions cannot be applied or if builds would run as LocalSystem.
Private state needs a local NTFS directory. Hosted content uses separate origins;
Caddy's unauthenticated admin API is disabled.

`MyHosting.exe` is the main launcher and contains the dashboard. Caddy and Node
are additional managed processes. Node is provisioned only for builds/server apps
that need it. Runtime archives are pinned to official versions and SHA-256 values.
No Docker or external database is required. Automatic public HTTPS still requires
valid DNS, reachable ports and successful certificate provisioning.

## Develop locally

Requirements: Go 1.27, Node supporting Vite 8, Windows and protected NTFS test
storage. Public source/dependency caches may use other volumes.

```powershell
cd web
npm ci
npm run build
cd ..
go test ./internal/hosting
go build -trimpath -o dist/MyHosting.exe ./cmd/myhosting
```

Use a fresh synthetic installation for testing. Never test against an existing
Workspace/MyAccounts/FileBrowser data directory. Build and release locally;
GitHub Actions are deliberately not used in this organization workflow.

## Licences

Original application and artwork: Apache-2.0. Caddy is a separately managed
Apache-2.0 dependency; Node and npm have their own included notices. React, Vite,
AUI compiler, Lucide, SQLite and Go dependencies retain their licences. See
[NOTICE](NOTICE); the release dependency inventory is generated during packaging.
