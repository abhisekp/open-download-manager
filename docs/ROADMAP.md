# Milestones and parity inventory

The target is an independent IDM/FDM/MDM-style desktop download manager with a first-class extension platform, as defined in [PRODUCT-BRIEF.md](PRODUCT-BRIEF.md). The selected screen establishes visual direction, not acceptance of a reduced final product. The following work remains explicit.

| Capability | Current milestone | Next acceptance gate |
|---|---|---|
| HTTP transfers | Native segmented/single-stream transfers, HTTPS Basic OS-keyring accounts, bounded automatic retries with durable backoff, queue fairness and crash recovery | Wider disk/origin/CDN/proxy and authentication coverage |
| Parallel transfer/resume | Three fixed native ranges for strong-validator resources ≥8 MiB, per-range crash checkpoints, safe changed-resource fallback and single-connection choice; optional aria2 parallel transfers | Dynamic splitting, mirrors and broader origin/CDN/platform compatibility |
| Torrents/magnets | Native Rain integration, metadata import, local peer transfer, persistence, permission-gated public tracker additions/atomic paused replacement, bounded opt-in seeding and live shared download/upload caps | File selection, priorities, ratio-based policies and public/private swarm matrix |
| FTP/SFTP | Engine URI path accepted | Real protocol/authentication tests and credential storage design |
| Queue | Persisted native HTTP ordering, concurrency/per-source-host limits, pause/resume and batch import | Unified torrent/aria2 queue groups and richer queue policies |
| Scheduling | Persisted one-time native HTTP UTC starts with local-time UI, paused/overdue/crash recovery and real byte verification | Recurring windows, torrent/aria2 schedules and OS wake integration |
| Speed controls | Persisted native HTTP/optional aria2 download limits and native torrent download/upload caps, enforced separately | Traffic modes and a shared application-wide budget |
| File management | Startup destination, retain files on removal, first-file SHA-256 verification | Categories, safe rename, move, collision and disk-full handling |
| History | Bounded durable history with recorded-time eviction, failed-write rollback, active-transfer protection and consented local JSON export with sensitive details excluded by default | Age-based retention policy UI, reviewed restore/import and crash window reduction |
| Browser integration | Paired bridge; installed Firefox/Chrome/Edge tests pass; Firefox submitted awaiting review; Edge submitted In review; Safari source | Store review, signed marketplace install tests, native messaging and installer-assisted handoff; Chrome registration payment blocked |
| Native desktop | Native window/tray, single instance, explicit background action and actual packaged runtime/transfer/quit tests on six x64/ARM64 targets | Native installer/update/uninstall integration, accessibility and notifications; 32-bit targets retain browser UI |
| Media extraction | Not implemented | Explicit provider adapters, rights review, safe dependency management |
| Extensions | Signed local WASI URL-resolver packages, SDK, isolated workers, permission review, lifecycle and real transfer integration | OS-enforced worker RSS limits, network provider APIs, events/UI contributions, automatic updates and independent hardening |
| Tracker management | Feed review, explicit additions and atomic paused public-torrent replacement/removal; stale/private/unknown-metadata refusal, failed-write preservation and persistence | Richer tracker status, tier editing and broader permission/network audit |
| Site explorer, previews, conversion | Not implemented | Separate scoped milestones |
| RSS and subscriptions | Not implemented | Bounded feed parsing, previewable rules, durable item identity/deduplication and backfill limits |
| Debrid and provider accounts | Not implemented | Explicit provider adapters, secure credential storage, quota/error handling and paid account use only when owner-authorized |
| Archives and IDM import | Not implemented | Safe opt-in extraction with traversal/link/size safeguards; reviewed import with sensitive URL/referrer/cookie handling and rollback |
| Network controls | Native transport safety and system transport defaults | User-managed proxy/DNS/adapter settings, credential handling and disconnect/leakage checks |
| Themes, localization and accessibility | Approved layout with persistent light/dark/system choices, live system updates, color tokens and reduced-motion preferences; semantic controls and basic focus containment | Translated messages/RTL, complete keyboard and screen-reader journeys |
| Diagnostics and CLI | Startup flags, local authenticated UI API; no automatic reports | User-reviewed redacted reports, stable versioned CLI/JSON contract and support/restore guidance |
| Remote MCP | Design only | User-bound API keys, hosted TLS, isolation and real client tests |
| Store/installers | Release preparation | Engine bundle/license review, MSIX identity, signing and clean installs |

No telemetry, accounts, payment service, AI dependency or mandatory hosted backend is required for the local manager. Cloud control would be an optional product with a separate security and operating-cost model.

Feature references rechecked on 2026-11-20: [MDM](https://misterdownloadmanager.com/features/), [IDM](https://www.internetdownloadmanager.com/features2.html), and [FDM](https://www.freedownloadmanager.org/features.htm). FDM labels site exploration, whole-site fetching, mirrors and partial ZIP operations as legacy features; they remain separate scope entries rather than claims about its current version or ODM. Historical/obsolete browser and OS support is not implied by “major browsers” or the agreed desktop architecture matrix. Comparative source claims are feature references, not independent security, compatibility or speed benchmarks.
