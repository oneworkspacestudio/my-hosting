# Site addresses and automatic publishing

## Recommended: free Cloudflare Tunnel

When Workspace already manages a Cloudflare connection, My Hosting uses that
connection. The installer passes its protected provider credentials only to the
Hosting control process, never into project builds or Node app environments.

For a zone such as `m8e.co.uk`:

- Production: `https://my-site-sites.m8e.co.uk`.
- Preview: `https://deploy-<10-character-id>--my-site-sites.m8e.co.uk`.
- Other installations use their own connected zone; this product does not grant
  other installations access to the `m8e.co.uk` DNS zone.

Flat addresses are first-level subdomains, covered by the zone's ordinary
Cloudflare Universal SSL certificate. The tunnel requires no inbound router
ports or static public IP. Domain registration and the Windows host remain the
operator's responsibility. Cloudflare service terms still apply.

Publishing checks the connected account/zone, creates an exact proxied CNAME and
an exact hostname route to Caddy's loopback site port, preserving the original
hostname. Existing routes, DNS records and tunnel settings are retained. A DNS
or route conflict fails with guidance; it is never silently overwritten. There
is no zone-wide wildcard tunnel rule. Sites run on separate origins from the
authenticated Workspace dashboard.

The release is marked published only after its expected deployment header is
observed over valid public HTTPS. Fresh Cloudflare public DNS and an IP-pinned
connection avoid Windows/router wildcard or negative-cache false failures. The
real hostname and normal public CA chain are still validated; no TLS bypass is
used. Local browser/router DNS caches may take their remaining TTL to refresh.
During publication, the build log reports DNS/tunnel/certificate checks.

## Optional: nested addresses and direct Caddy HTTPS

Choose **nested** addresses with parent `sites.m8e.co.uk` for
`my-site.sites.m8e.co.uk`. In **direct** transport, point wildcard DNS at the
server and forward inbound TCP80/443 to Caddy. Caddy obtains individual public
certificates for configured site hostnames. This is optional, not the default
for users with an existing Workspace tunnel.

A tunnel can also carry nested addresses, but Cloudflare's standard Universal
SSL in a full zone does not cover that extra level. Arrange suitable edge
certificate coverage separately; My Hosting never purchases an add-on.

## Caddy management on Windows

Official Caddy2.11.7's Unix socket listener attempts a POSIX chmod that fails on
Windows AF_UNIX socket files. My Hosting therefore uses its authenticated remote
admin API on an ephemeral loopback TCP port. The plaintext/local admin endpoint
is disabled. A dedicated client certificate/private key is protected by DPAPI
and private NTFS ACLs; only POST `/load` is allowed for that client. The server
identity is issued by a dedicated Caddy internal CA, trusted only by this
control client. No Windows/browser trust store is changed. Project workers
cannot read the client credentials, and connections without a certificate fail.

Routing changes use Caddy validation plus its atomic load API and an exact
configuration-revision health probe. A failed switch restores the previous
configuration. Caddy and Node processes stop for a consistent encrypted backup
and resume afterward; restored installations leave public routing disabled.

Sources: [Cloudflare SSL coverage](https://developers.cloudflare.com/ssl/edge-certificates/universal-ssl/limitations/),
[Cloudflare Tunnel](https://developers.cloudflare.com/tunnel/),
[Caddy management API](https://caddyserver.com/docs/api).


## Publishing the managed update contract

The current installed Hosting group contains `hosting`, `hosting-LICENSE`,
`hosting-NOTICE` and `hosting-THIRD-PARTY-LICENSES.txt`. Supplemental guides,
compatibility and dependency inventories are in the app release ZIP; current
optional-app selection does not retain those supplemental catalog packages.
Generate `workspace-update.json` for the four installed packages, with the
canonical catalog app/launch contract (default9007), not a test installation's
allocated port or the raw supplemental package list. The updater preserves each
installation's actual port. Do not weaken its package/path ownership checks.

Use the suite's `cmd/update-manifest` publisher against a compatible canonical
catalog scoped to those installed files. Each package names the published ZIP
and its exact member, hash and size. Upload the marker with the app release and
verify its actual GitHub digest. Native acceptance downloaded the real public
ZIP through that contract and verified the extracted executable. Paired suite
publication then makes first-install discovery available in its normal catalog.
