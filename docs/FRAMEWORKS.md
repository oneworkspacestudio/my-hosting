# Native Windows framework builds

Builds run in a restricted Windows token and a Job Object. They do not inherit
the manager's environment, GitHub token, OIDC secret or Caddy credentials. This
is intended for trusted administrators and trusted source, not hostile tenants.

## Next.js

The real `examples/next` fixture builds Next 16.4.2 standalone output and serves
a server-rendered timestamp through Caddy using managed Node 24.22.0. It enables
`experimental.workerThreads`, disables `webpackBuildWorker` and uses one build
CPU. Include those options alongside `output: 'standalone'` in the tested config.
The adapter copies static/public assets into standalone output and retains only
that self-contained runtime directory in the immutable release.

## Nuxt

The real `examples/nuxt` fixture builds Nuxt 4.6.2 with Nitro's `node-server`
preset. On Windows its documented `rollup:before` hook replaces the esbuild
transform plugin with the pinned in-process `oxc-transform` compiler. Copy the
example configuration and dependency when building Nuxt in this worker. The
adapter retains only `.output`, then starts `.output/server/index.mjs`.

Nuxt and Next have both been built and served through Caddy in fresh protected
test directories. These examples are compatibility configurations, not a claim
that arbitrary framework versions or every plugin works without changes.

## Child-process compatibility

Windows' default named-pipe security descriptor does not grant our per-project
restricting SID write access. Node child processes with inherited or ignored
stdio work; tools requesting new piped stdio can fail with `spawn EPERM`. The
worker deliberately keeps its restrictions. Use an in-process/worker-thread
compiler configuration as shown above, or upload prebuilt runtime output.
My Hosting does not silently retry a failed build with broader permissions.

This behavior follows the documented [Windows named-pipe default permissions](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights).
The Nuxt configuration uses [Nitro's bundler hook](https://nitro.build/docs/modules)
and the [Oxc transform API](https://oxc.rs/docs/guide/usage/transformer).

## Static and generic Node

Static ZIP output does not need Node. npm builds use the configured managed
Node major. Generic Node projects retain their source and installed dependencies
because the manager cannot infer which files a custom start command requires.
They must listen on `127.0.0.1` and the supplied `PORT`, keep mutable files in
`MYHOSTING_DATA_DIR`, and avoid writing into immutable release directories.
