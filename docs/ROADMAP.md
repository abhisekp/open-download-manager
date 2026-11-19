# Milestones and parity inventory

The target is an independent IDM/FDM/MDM-style desktop download manager with a first-class extension platform, as defined in [PRODUCT-BRIEF.md](PRODUCT-BRIEF.md). The selected screen establishes visual direction, not acceptance of a reduced final product. The following work remains explicit.

| Capability | Current milestone | Next acceptance gate |
|---|---|---|
| HTTP transfers | Native fallback without aria2, TLS/redirect/Range/resource-change/interruption/checkpoint/collision tests, real pause/resume/integrity integration | Authentication UI/secure credential storage, segmented native transfers and wider server/platform compatibility |
| Parallel transfer/resume | Optional aria2 four-connection transfers; native HTTP single-stream validator-aware resume and crash checkpoints | Native segmentation, broader parallel-server matrix and interrupted finalization recovery evidence |
| Torrents/magnets | Native Rain integration, metadata import, local peer transfer, persistence and permission-gated public tracker additions | File selection, priorities, tracker removal/replacement, upload limits and public/private swarm matrix |
| FTP/SFTP | Engine URI path accepted | Real protocol/authentication tests and credential storage design |
| Queue | Engine concurrency limit, pause/resume, batch URL import | Reordering, per-host policies, queue groups |
| Scheduling | Not implemented | Persisted schedules, time zones, sleep/wake and missed-run policy |
| Speed controls | Persisted native HTTP and optional aria2 download limits, enforced separately | Native torrent download/upload limits, traffic modes and a shared application-wide budget |
| File management | Startup destination, retain files on removal, first-file SHA-256 verification | Categories, safe rename, move, collision and disk-full handling |
| History | Bounded durable JSON history and forget | Export, retention policy UI and crash window reduction |
| Browser integration | Paired bridge; installed Firefox test/CI and Mozilla package validation pass; Chromium packages and Safari source | Firefox final submission/review, actual Chromium/Edge tests/listings, native messaging and installer-assisted handoff; Chrome registration payment blocked |
| Native desktop | Native window/tray, single instance, explicit background action and actual packaged runtime/transfer/quit tests on six x64/ARM64 targets | Native installer/update/uninstall integration, accessibility and notifications; 32-bit targets retain browser UI |
| Media extraction | Not implemented | Explicit provider adapters, rights review, safe dependency management |
| Extensions | Signed local WASI URL-resolver packages, SDK, isolated workers, permission review, lifecycle and real transfer integration | OS-enforced worker RSS limits, network provider APIs, events/UI contributions, automatic updates and independent hardening |
| Tracker management | Feed review plus explicit public-torrent additions; private/unknown-metadata refusal, duplicate checks and persistence | Removal/replacement, atomic replacement/rollback, richer tracker status and permission audit |
| Site explorer, previews, conversion | Not implemented | Separate scoped milestones |
| Remote MCP | Design only | User-bound API keys, hosted TLS, isolation and real client tests |
| Store/installers | Release preparation | Engine bundle/license review, MSIX identity, signing and clean installs |

No telemetry, accounts, payment service, AI dependency or mandatory hosted backend is required for the local manager. Cloud control would be an optional product with a separate security and operating-cost model.
