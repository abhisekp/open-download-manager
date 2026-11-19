# Development verification — 2026-11-19

This report covers local development evidence, not production or Store readiness. The public repository is https://github.com/abhisekp/open-download-manager. The user chose an independent implementation, MIT licensing, public source, and the final navy/cyan icon. MDM source and assets were not reused.

## Recorded CI checkpoints

| Scope | Exact signed source | Passing run |
|---|---|---|
| Core race/integration checks, nine portable targets, Debian installation and native platform tests | `e059994` | [Verify and build](https://github.com/abhisekp/open-download-manager/actions/runs/44478683534) |
| Actual packaged desktop runtime, UI, real transfer and clean quit on Windows/Linux/macOS x64/ARM64 | `e059994` | [Native desktop candidates](https://github.com/abhisekp/open-download-manager/actions/runs/44478683558) |
| Installed Firefox companion: pairing, real transfer bytes, revocation, disconnect and uninstall | `e059994` | [Installed browser companion](https://github.com/abhisekp/open-download-manager/actions/runs/44478683749) |
| Portable release validation, complete packages, official CycloneDX schema, provenance and inventory | `39cbf86b1a482a50f7e6a019cbd2d0270028c7c9` | [Validation only](https://github.com/abhisekp/open-download-manager/actions/runs/44477177969) |
| Three development MSIX packages, Windows x86 runtime and x64 install/launch/upgrade/uninstall | `39cbf86b1a482a50f7e6a019cbd2d0270028c7c9` | [MSIX validation only](https://github.com/abhisekp/open-download-manager/actions/runs/44477173324) |

The experimental `v0.1.0-dev` release at signed source `979322938604eeafda854eafb944a27a01027b65` is public. [Its gated publication run](https://github.com/abhisekp/open-download-manager/actions/runs/44455684894) passed both owner reviews. Public packages were downloaded and checked with the verifier from that tag. Later source features are not in those published binaries. Changing development VERSION to `0.2.0-dev` does not publish another release.

## Local and integration evidence

- Native HTTP without aria2: real fixture transfers, pause/resume and matching bytes; validator/resource-change/range/TLS/redirect/stall checks, crash checkpoints, interrupted finalization, safe collisions and persisted speed limits. Optional-engine and corrupt add-on/torrent components do not have to hide a working native HTTP engine.
- Native Rain BitTorrent: two loopback peers, magnet metadata exchange, verified pieces and exact file bytes, paused/completed state normalization and database recovery. No public swarm was contacted.
- API capability boundaries: UI bearer/Host/Origin checks, browser-token isolation and revocation, bounded batch/history/checksum operations and storage path boundaries.
- Signed installable WASI resolvers: exact package/signature/module/license verification, disabled-by-default permission review, publisher continuity, update revocation, restart persistence, worker time/memory/output bounds, and filesystem/network/environment denial. The actual package-to-API-to-reviewed-link-to-real-download flow passes. Go's worker memory target is not an OS RSS ceiling.
- Native desktop: source and packaged code plus actual renamed runtime launches; isolated renderer, real local transfer, hide/show, saved state and clean exit. Linux package paths avoid the confirmed upstream Chromium setuid-launcher space bug. Windows/macOS publisher signing and macOS notarization are not implied.
- Firefox 160.0.1/geckodriver 0.37.1 on Windows: exact companion 0.2.0 package `9e2a9de6432bda52735154b4a859dc2322dffc8db88c0feb841e441c6a950d06` temporarily installed and tested against both development ODM and downloaded/checksum-verified public ODM 0.1.0-dev with aria2. Mozilla validated it with zero errors/warnings. The owner approved the public submission, and Mozilla confirmed version receipt; signing/publication remain unverified until read back. The supplied source reproduced all 11 shipped file bytes on Windows and Linux.
- Public tracker additions: local race tests cover permission refusal, private/unknown-metadata protection, whole-input validation, duplicate/concurrent edits, unchanged info hashes, a real local tracker and restart persistence. A simulated storage failure verifies honest partial-result reporting. Actual Chrome UI tests cover review gating, approval clearing after URL edits, successful additions, and private/unknown-metadata read-only behavior. The external feed was mocked for the separate feed-to-review-to-real-addition flow. These new tracker changes require their own post-push CI outcome; the older CI links above do not attest them.
- Go tests/race/vet and official govulncheck v1.10.0 passed for the tracker source locally; no vulnerabilities were reported. UI build/Sites checks, WebExtension contracts and package-version tests passed. Dependency scan results are point-in-time evidence, not a guarantee of complete security.

## Distribution and remaining acceptance

Nine portable Go targets are built: Windows x86/x64/ARM64, Linux x86/x64/ARMv7/ARM64 and both Macs. Six x64/ARM64 native desktop targets are separate development candidates; current Electron does not provide the three 32-bit runtime variants. Their portable browser-interface builds remain available. Archive checks are distinct from native execution, emulation, installation and certification.

The native desktop candidates are not yet integrated into the portable/MSIX publication contract. Their runtime inventory/provenance, native installers, upgrades and clean uninstall remain release work. MSIX installation uses a disposable test certificate only on a hosted runner; no test trust root is installed on the owner's computer. Store identity/certification/submission remain unresolved. Apple Store publication and paid Apple Developer membership are excluded by the owner; direct macOS signing/notarization limitations remain explicit.

Chrome registration is payment-blocked. Edge publisher content has not rendered successfully. Firefox is submitted, not yet verified publicly installable. Safari is conversion source only. Native messaging, installer-assisted marketplace handoff, automatic interception and browser credential handoff remain open.

ODM is not complete IDM/FDM/MDM parity or production-ready. Native segmentation/authenticated credentials, scheduling/queue organization, richer extension APIs/updates and hard worker memory limits, torrent selection/priorities/seeding/limits, media providers and full distribution remain on [the roadmap](ROADMAP.md). No source push creates a release automatically, and no passing test establishes an unimplemented feature.
