# Native acceptance examples

These small, synthetic projects exercise real framework builds rather than
pre-rendered adapter mocks. Framework versions and package locks are pinned.

For `next`, use the Next adapter, Node24, install arguments
`["npm","ci","--include=dev","--ignore-scripts"]`, build
`["npm","run","build"]`, memory3072MiB, disk4096MiB and timeout1800seconds.
The included config produces Next's standalone server. My Hosting copies its
static/public assets into the standalone output before freezing the artifact.

Run native tests with verified official Caddy/Node ZIPs in
`MYHOSTING_RUNTIME_CACHE` (caddy.zip/node24.zip). Set `MYHOSTING_SSR=1` to include
the larger, network-dependent framework builds. Never target owner installations.
