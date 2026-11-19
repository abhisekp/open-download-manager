Open Download Manager 0.2.0-dev — experimental candidate.

Adds built-in HTTP/HTTPS transfers when aria2 is absent, with validator-aware resume, durable checkpoints, safe filename collisions, a three-transfer queue and persisted speed limits. BitTorrent continues to run natively through the MIT-licensed Rain library. Optional separately installed aria2 adds accelerated file transfers and FTP/SFTP; it is not bundled. DHT/PEX remain disabled and torrent seeding stops on completion.

Adds signed local WASI URL-resolver add-ons, explicit publisher/package and host-permission review, disabled-by-default installation, isolated worker execution, resolution review before download, revocation and uninstall. This is the first resolver SDK, not the complete extension platform. Filesystem/network/environment access is not granted to these modules. Go memory targets are not OS-enforced worker memory ceilings.

Recovers recognized data ownership after crashes and isolates optional engine/add-on startup failures so ordinary native HTTP downloads can remain available. Existing local batch import, history, checksum checks and paired link-submission extensions remain available.

Portable packages cover Windows x86/x64/ARM64, Linux x86/x64/ARMv7/ARM64 and macOS Intel/Apple Silicon. They use the local browser interface. A native window/tray application is being validated and packaged separately; those desktop candidates are not silently included in this portable release or the development MSIX pipeline. macOS direct downloads are not Apple Developer notarized; Apple Store publication is excluded by the owner.

Browser packages are development/manual-install artifacts. Firefox packages require Firefox 142 or later and disclose selected-URL transmission to the local app. They are not marketplace-signed releases. Chrome registration is blocked by the owner's payment issue. No installer automatically installs browser extensions.

Portable release assets include checksums, a CycloneDX dependency inventory and GitHub/Sigstore build attestations. These establish artifact/source relationships; they are not Windows Authenticode signatures or Microsoft Store certification. No paid signing service is configured.

ODM is not complete IDM/FDM/MDM feature parity or a production-ready stable release. Advanced torrent controls, authenticated downloads, broader extension APIs, native desktop distribution, marketplace listings, additional installer formats and update delivery remain open. See `docs/PRODUCTION-READINESS.md` for acceptance boundaries.
