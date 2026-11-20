# Production acceptance — in progress

ODM is **not complete or production-ready**. The independent IDM/FDM/MDM product brief remains the target. A passing development build, a signed source tag or an experimental prerelease must not be presented as full product parity.

## Delivery milestones

| Milestone | Current implementation | Required acceptance |
|---|---|---|
| Reliable transfer core | Native torrents and HTTP/HTTPS, including three fixed ranges with per-range crash checkpoints and changed-resource isolation; optional aria2; local real-process integrity evidence | Same-revision segmented platform CI, authentication/credential storage, automatic retries, broader disk-full/origin/CDN matrix and real FTP/SFTP verification |
| Desktop application | Native window/tray, single instance and explicit background/quit; actual packaged runtime/transfer/quit CI passed on all six x64/ARM64 targets at `e059994` | Native install/update/uninstall, accessible keyboard journeys and notifications; Linux install paths must avoid spaces |
| Add-on foundation | Signed local WASI resolvers, reviewed permissions, isolated workers, lifecycle/persistence and real resolver-to-download handoff; native tests passed | OS-enforced worker RSS limits and independent hardening |
| Complete extension platform | Resolver SDK/API 1 and built-in tracker feed | Controlled provider networking/credentials, events/UI contributions, compatibility policy, richer examples and update distribution |
| Download organization | Native HTTP persisted queue order/concurrency/per-host limits and one-time schedules; real UI, crash/overdue/restart and integrity checks pass locally; batch import, search and history | Post-push platform acceptance, unified torrent/aria2 queues, recurring windows/OS wake, categories, safe file organization and export/retention |
| Torrent controls | Magnets, import, local peer transfer, pause/resume, persistence and public tracker additions; private/unknown-metadata protection verified locally | Selection/priorities, tracker removal/replacement/rollback, limits and public/private swarm matrix |
| Browser integration | Exact 0.2.0 Firefox/Chrome/Edge installed companion checks passed; owner-approved Firefox submission is Awaiting Review and Edge is In review; approved icon and two actual companion screenshots saved in each listing | Store certification/signing/public installation, native messaging, automatic interception/credential handoff and installer integration; Chrome registration payment-blocked |
| Platform distribution | Nine portable targets including both macOS architectures; Windows/Linux runtime tests and both Mac runtime jobs passed at `f1faaac`; Debian and development MSIX | Native desktop execution evidence, clean install/update/uninstall, appropriate installers, Windows signing/certification and public artifact read-back |

The requested desktop matrix is Windows x86/x64/ARM64, Linux x86/x64/ARMv7/ARM64, and macOS Intel/Apple Silicon. This does not imply mobile OS support, obsolete Windows ARM32, every Linux distribution, or certification on untested OS versions. Current CI operating systems and actual run outcomes determine verification claims.

## Release boundaries

The owner approved signed tag `v0.1.0-dev` at `979322938604eeafda854eafb944a27a01027b65`, draft preparation and an approval-gated experimental publication workflow. New product changes on main are outside that exact tagged build. Protected release reviews remain required; a development prerelease is not a stable production release.

That approved experimental prerelease is public: [v0.1.0-dev](https://github.com/abhisekp/open-download-manager/releases/tag/v0.1.0-dev). Both protected reviews and publication succeeded in run `44455684894`; the public assets were downloaded and the tagged package/checksum verifier passed. Signed WASI add-ons, crash-safe lock recovery, native HTTP and the native desktop candidate were added after that tag and are not in those published binaries.

The owner confirmed that ODM has no Microsoft Store name reservation and no Windows signing certificate/service. Windows developer enrollment has not been established either way; the earlier question did not ask about enrollment. Verify the existing Partner Center account before requesting registration. Store identity, submission and certification remain incomplete. Store MSIX signing is provided by Microsoft after certification. The owner excludes Apple Store publication and declines paid Apple Developer membership. macOS remains a direct-download target with explicit signing/notarization limitations; no paid Apple enrollment or Apple Store submission is authorized. Edge Add-ons and Firefox publisher accounts are available and their use is authorized. Chrome Web Store registration is blocked: the owner confirmed that their cards cannot pay its registration fee. Keep its manual-install package available; do not claim a Chrome listing or try purchases/payment changes. No paid service or signing purchase is authorized.

## Current validation approach

Run unit, race, API permission and real-engine tests after each implementation slice. Extension tests compile real WASI modules and exercise tampering, publisher continuity, grant revocation, persistence, host restrictions, memory rejection, runaway cancellation, and filesystem/network/environment denial. Real API integration signs and installs a fixture, reviews/resolves a link, explicitly starts a real transfer, then revokes and uninstalls it. Browser checks cover the user-facing review and lifecycle flow. Target artifacts must include dependency notices and pass package/checksum/provenance checks.

Source work, configured workflows, queued jobs, passing jobs, uploaded candidates, installed-app tests and published artifacts are separate evidence states. Keep unimplemented product capabilities and blocked account/certification requirements visible until their acceptance criteria actually pass.
