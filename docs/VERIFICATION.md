# Development verification — 2026-11-19

This report covers local development evidence, not production or Store readiness. The public repository is https://github.com/abhisekp/open-download-manager. The user chose an independent implementation, MIT licensing, public source, and the final navy/cyan icon. MDM source and assets were not reused.

## Passed locally on Windows x64

- Go unit/integration tests and race detection, including API token/Host/Origin boundaries, RPC errors, extension endpoint restrictions, tracker parsing, browser-token isolation/revocation, persistent history, checksum mismatch and path confinement.
- Real HTTP fixture transfers: add, pause, resume, byte-for-byte SHA-256 integrity, duplicate removal for batch input, local browser-bridge submission/revocation and aria2 bandwidth limits.
- Restart tests: paused transfers and completed history recover; archived checksum checks and forgetting history work; lock and temporary engine credential files are cleaned on graceful exit.
- Native Rain BitTorrent transfer between two loopback peers: magnet metadata exchange, verified piece transfer, exact downloaded bytes, native database recovery and unsafe metadata rejection. No public swarm was contacted.
- Native-only application startup/import with a deliberately missing aria2 executable. Other protocols report the missing optional engine.
- Official Go vulnerability scanner v1.10.0: no vulnerabilities found for the scanned source. npm audit after compatible build-tool fixes: zero reported vulnerabilities.
- UI production build, four bundled worker/packaging tests, and three WebExtension connection contract tests.
- Browser UI: real transfer display, validation feedback, live tracker retrieval (75 entries at the time), permission controls, custom checksum dialog with a successful match, and custom remove confirmation. Removal retains files.
- Dependency license texts collected from actual Go build modules; the old unlicensed helper was replaced with original MIT compatibility code and tested separately.

## Packaging and remote boundaries

Windows x86/x64/ARM64 and Linux x86/x64/ARMv7/ARM64 build recipes are provided, together with ZIP, tar.gz, Debian and WebExtension packaging. Cross-compilation and archive/PE/ELF inspection do not establish runtime compatibility on the other CPUs. All seven native-engine targets cross-compiled locally. Windows x86/x64/ARM64 PE headers, Linux ELF headers, archives and checksums are inspected separately; authoritative CI results belong to the matching Git commit.

The Chromium/Firefox packages are development artifacts. Installed-browser tests, marketplace review/signing/publication and Safari conversion remain pending. Browser cookies are not forwarded and automatic interception is not implemented. No marketplace URLs or IDs are fabricated.

Windows SDK makeappx 10.0.26100.9457 validated x86/x64/ARM64 packages using an explicit development identity. The preparation workflow accepts the real reserved identity as input. Packages are unsigned review artifacts, not certified Store releases. Store identity, install/update/uninstall tests, optional HTTP-engine bundling, signing/certification and submission remain pending. Store submission is not automated in this milestone.

Public repository creation was verified through the browser and GitHub connector. Mockend access was explicitly approved after automatic review blocked the initial creation attempt. Source commits require actual signature verification; see Git history and the remote workflow run for the published checkpoint. No tag or public binary release is automatically created by a source push.

## Open product work

This is an early browser-based local app, not a complete FDM/MDM replacement or a native desktop window. Scheduling, queue reordering, complete durable preferences, browser credential handoff, third-party installable add-ons, media/gallery adapters, native torrent file priorities/seeding controls, remote MCP, and full release certification remain on the roadmap. A passing test suite does not establish untested features.
