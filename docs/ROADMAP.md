# Milestones and parity inventory

The target is an independent IDM/FDM/MDM-style desktop download manager with a first-class extension platform, as defined in [PRODUCT-BRIEF.md](PRODUCT-BRIEF.md). The selected screen establishes visual direction, not acceptance of a reduced final product. The following work remains explicit.

| Capability | Current milestone | Next acceptance gate |
|---|---|---|
| HTTP transfers | Local add/pause/resume/integrity test | TLS, redirects, authentication and network interruption matrix |
| Parallel transfer/resume | aria2 configured for four connections | Verify server Range fallback, ETag changes and crash recovery |
| Torrents/magnets | Native Rain integration, metadata import, local peer transfer and persistence tests | File selection, priorities, tracker editing, upload limits, public/private swarm matrix |
| FTP/SFTP | Engine URI path accepted | Real protocol/authentication tests and credential storage design |
| Queue | Engine concurrency limit, pause/resume, batch URL import | Reordering, per-host policies, queue groups |
| Scheduling | Not implemented | Persisted schedules, time zones, sleep/wake and missed-run policy |
| Speed controls | Session-wide download limit | Upload limit, traffic modes and persistent preferences |
| File management | Startup destination, retain files on removal, first-file SHA-256 verification | Categories, safe rename, move, collision and disk-full handling |
| History | Bounded durable JSON history and forget | Export, retention policy UI and crash window reduction |
| Browser integration | Paired link-submission bridge, Chromium/Firefox packages, Safari conversion source | Actual browser tests, marketplace signing/publication and installer-assisted store handoff |
| Native desktop | Not implemented | Native shell, tray, accessibility, single instance, shutdown UX |
| Media extraction | Not implemented | Explicit provider adapters, rights review, safe dependency management |
| Extensions | One built-in tracker-feed capability | First-class SDK and versioned host API, isolated execution, package installation/lifecycle, capability enforcement, UI/events, signing, upgrades and compatibility tests |
| Tracker management | Feed fetch/copy only | Per-torrent changes after private flag checks; rollback and permission audit |
| Site explorer, previews, conversion | Not implemented | Separate scoped milestones |
| Remote MCP | Design only | User-bound API keys, hosted TLS, isolation and real client tests |
| Store/installers | Release preparation | Engine bundle/license review, MSIX identity, signing and clean installs |

No telemetry, accounts, payment service, AI dependency or mandatory hosted backend is required for the local manager. Cloud control would be an optional product with a separate security and operating-cost model.
