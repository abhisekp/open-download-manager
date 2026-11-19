Experimental Open Download Manager development prerelease, 0.1.0-dev.

Includes a local browser interface, native MIT-licensed Rain BitTorrent support, transfer controls, live progress, batch link import, persistent history, SHA-256 verification, session speed limits, a permission-gated tracker feed, and paired browser-extension development packages. Non-torrent protocols use separately installed aria2; it is not bundled. DHT/PEX are disabled and torrent seeding stops on completion.

Portable packages target Windows x86/x64/ARM64 and Linux x86/x64/ARMv7/ARM64. Debian packages and Chromium/Firefox extension packages are included, along with Safari conversion source. CI records native ARM64 tests, x86 startup checks, explicitly emulated ARMv7 checks, real transfer/recovery tests and package installation checks. These checks do not establish compatibility with every supported operating-system version or public torrent swarm.

Packages include SHA-256 checksums, a CycloneDX dependency inventory and GitHub/Sigstore build attestations. These verify artifact/source relationships; Windows executables are not Authenticode-signed. MSIX validation artifacts are separate from this portable release and are not a Microsoft Store submission.

This is an early implementation, not complete IDM/FDM/MDM feature parity. A native desktop shell, the full third-party add-on platform, browser marketplace listings and several advanced download-management capabilities remain outstanding. There is no automatic engine or browser-extension installation. See the product brief, roadmap and release guide before use.
