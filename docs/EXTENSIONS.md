# Application extensions

ODM now supports locally installed, signed WASI `url-resolver` packages. The Extensions page provides exact-package permission review, installation, enable/disable, uninstall and explicit resolution-to-download handoff. Registry/grants persist locally; updates require review and revoke the prior grant. Publisher-key changes cannot replace an installed ID.

Each invocation runs in a separate worker process, using a WASM interpreter with no mounted files, inherited environment, network sockets or subprocess imports. The host enforces declared input/output hosts, integrity checks, output/memory limits and deadlines. API 2 additionally exposes a [bounded public HTTPS host function](../sdk/NETWORK.md) after separate per-link/exact-package approval. API 1 retains no network permission. See [SDK contract, build examples, limits and verification](../sdk/README.md).

This is a concrete first platform milestone, not the complete product brief's extension ecosystem. Provider-specific adapters, credential brokering, event/UI contributions, automatic update distribution, OS-enforced worker RSS limits and broader independent hardening remain outstanding. Core downloads are available in portable browser UI and native x64/ARM64 desktop candidates; native installer/updater acceptance remains separate.

## Authenticated host API

All routes use the existing local UI bearer token and Host/Origin guard. Browser pairing tokens cannot call these routes.

| Route | Operation |
|---|---|
| GET `/api/addons` | List installed signed manifests and grants |
| POST `/api/addons/inspect` | Inspect a raw .odmx file without installing it |
| POST `/api/addons/install` | Install the reviewed file; requires `X-ODM-Approve-SHA256` matching its digest |
| POST `/api/addons/enable` | Persist `{id, enabled}` |
| POST `/api/addons/uninstall` | Revoke and remove `{id}` |
| POST `/api/addons/resolve` | Run `{id, url}`; API 2 network permissions additionally require `networkApproved:true` and the current package `digest`. Return approved-host URL suggestions; never automatically starts a transfer |

No generic RPC, command execution or remote add-on installation endpoint is exposed.

## Built-in tracker feed

The existing newTrackon feed is separate. It fetches `https://newtrackon.com/api/live` only after the page-session checkbox grant. HTTPS requests are timed out, redirects are rejected and responses are limited to 256 KiB / 500 validated unique tracker entries. Fetching never changes torrent trackers. Private torrent metadata must not be rewritten to bypass tracker restrictions.
