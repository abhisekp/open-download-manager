# Production acceptance — in progress

ODM is **not complete or production-ready**. The independent IDM/FDM/MDM product brief remains the target. A passing development build, a signed source tag or an experimental prerelease must not be presented as full product parity.

## Delivery milestones

| Milestone | Current implementation | Required acceptance |
|---|---|---|
| Reliable transfer core | Native torrents and fallback HTTP/HTTPS; optional aria2 for parallel HTTP/FTP/SFTP; checkpointed native resume and safe file finalization | Native HTTP platform evidence, authenticated-download UI/credential storage, native segmentation, disk-full matrix and real FTP/SFTP verification |
| Desktop application | Local authenticated browser UI | Native window/tray, single instance and crash recovery, accessible keyboard journeys, notifications and clear shutdown behavior |
| Add-on foundation | Signed local WASI URL resolvers, reviewed host permissions, isolated worker processes, persistence and install/disable/update/uninstall | Native platform tests and explicit real download handoff; OS-enforced worker RSS limits and independent hardening remain open |
| Complete extension platform | Resolver SDK/API 1 and built-in tracker feed | Controlled provider networking/credentials, events/UI contributions, compatibility policy, richer examples and update distribution |
| Download organization | Basic queue, batch import, search and history | Persisted schedules/settings, queue order/groups, categories, safe file organization, export/retention and failure recovery |
| Torrent controls | Magnets, file import, local peer transfer, pause/resume and persistence | Selection/priorities, tracker editing with private-torrent protection, limits and public/private swarm matrix |
| Browser integration | Paired Chromium/Firefox development packages; Safari conversion source | Installed-browser tests, native messaging/installer integration, actual marketplace signing and listings |
| Platform distribution | Windows/Linux portable builds, Debian and development MSIX; macOS builds/runtime jobs being added | Native execution evidence, clean install/update/uninstall, appropriate installers, signing/notarization/certification and public artifact read-back |

The requested desktop matrix is Windows x86/x64/ARM64, Linux x86/x64/ARMv7/ARM64, and macOS Intel/Apple Silicon. This does not imply mobile OS support, obsolete Windows ARM32, every Linux distribution, or certification on untested OS versions. Current CI operating systems and actual run outcomes determine verification claims.

## Release boundaries

The owner approved signed tag `v0.1.0-dev` at `979322938604eeafda854eafb944a27a01027b65`, draft preparation and an approval-gated experimental publication workflow. New product changes on main are outside that exact tagged build. Protected release reviews remain required; a development prerelease is not a stable production release.

Microsoft Partner Center registration/name reservation are not set up, as confirmed by the owner. Store submission and certification cannot be completed without those owner steps. Store MSIX signing is provided by Microsoft after certification. The owner excludes Apple Store publication and declines paid Apple Developer membership. macOS remains a direct-download target with explicit signing/notarization limitations; no paid Apple enrollment or Apple Store submission is authorized. Chrome Web Store, Edge Add-ons and Firefox publisher accounts are available and their use is authorized. No paid service or signing purchase is authorized.

## Current validation approach

Run unit, race, API permission and real-engine tests after each implementation slice. Extension tests compile real WASI modules and exercise tampering, publisher continuity, grant revocation, persistence, host restrictions, memory rejection, runaway cancellation, and filesystem/network/environment denial. Real API integration signs and installs a fixture, reviews/resolves a link, explicitly starts a real transfer, then revokes and uninstalls it. Browser checks cover the user-facing review and lifecycle flow. Target artifacts must include dependency notices and pass package/checksum/provenance checks.

Source work, configured workflows, queued jobs, passing jobs, uploaded candidates, installed-app tests and published artifacts are separate evidence states. Keep unimplemented product capabilities and blocked account/certification requirements visible until their acceptance criteria actually pass.
