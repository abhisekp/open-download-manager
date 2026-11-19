# Microsoft Store submission preparation

Status: blocked on Partner Center registration and name reservation. The owner confirmed on 2026-11-19 that neither is set up. No account was created, agreement accepted, fee paid or submission sent by this work.

## Required owner setup

1. Register as a Windows app developer in Partner Center and complete Microsoft's identity verification and developer agreement.
2. Reserve the intended product name. Availability of “Open Download Manager” has not been verified.
3. Copy the package identity Name, Publisher and Store product ID from the reserved product. These are identifiers, not credentials; never substitute development values.
4. Review the actual product description, privacy/support pages, age ratings, capabilities and distribution choices. Upload genuine screenshots of the packaged app; existing design mock-ups are not certification evidence.
5. Build the reserved identity packages through the protected packaging workflow, run certification and installed-app tests on a supported Windows client, and create the first submission in Partner Center.
6. After first publication, configure subsequent update automation with a Partner Center-associated Entra tenant and least-privilege application access. Enter credentials through protected GitHub environment secrets, never source or chat.

Microsoft handles signing of Store-distributed MSIX packages after certification. No purchased certificate is required for this route. Direct download distributions have separate signing requirements. Disposable self-signed CI test certificates are not release certificates.

## Listing text prepared for owner review

Product: Open Download Manager

Short description: An open-source download manager with native BitTorrent support and a local browser interface.

Description: Manage file downloads and BitTorrent transfers from a local interface. Import torrent files and magnets, pause and resume transfers, import multiple links, inspect progress and verify completed files with SHA-256. Pair a compatible browser companion extension to send selected download links to your local manager. HTTP/HTTPS and BitTorrent are built in. Optional separately installed aria2 adds accelerated file transfers and FTP/SFTP; it is not included. The current MSIX pipeline packages the local browser interface; the native window/tray candidate is being validated separately. It is an independent project and is not affiliated with Internet Download Manager, Free Download Manager or Mister Download Manager.

Support: https://github.com/abhisekp/open-download-manager/issues

Source/license: https://github.com/abhisekp/open-download-manager — MIT application code; bundled dependency notices accompany packages.

Do not advertise full IDM/FDM/MDM parity, a complete third-party add-on platform, automatic browser-extension installation, marketplace availability, tested FTP/SFTP authentication or certified Store compatibility. These claims are not established by the current implementation.

## Privacy statement prepared for owner review

ODM stores transfer sessions, downloaded files, torrent state and limited transfer history on the user's computer. Source URLs and paths may be retained in that local data. Browser pairing creates a local, revocable token. Keep the application data directory and launch link private.

The current application includes no account service, payment service or built-in analytics. Its interface and download-control API listen on the local loopback address. Download sources, BitTorrent peers and trackers necessarily receive network requests and may observe the user's network address and requested content. Fetching the optional live tracker feed contacts newtrackon.com. A separately installed aria2 process handles its supported transfers when available; native HTTP works without it.

Removing a transfer or forgetting history does not delete downloaded files. Do not include private URLs, launch links, pairing tokens or personal file paths in public issue reports. This statement describes the current development implementation; the owner must approve its public use and keep it synchronized with product changes.

## Submission limits

The Microsoft Store Developer CLI automates updates after the first publication. Creating the initial submission, identity verification, age ratings, account agreements and Microsoft certification cannot be honestly marked complete from packaging tests alone. No authenticated Store submission workflow is enabled until the reserved app and first-submission state are known; a publishing command can discard pending metadata drafts, so it must not be added as an unreviewed generic step.

Official references, checked 2026-11-19:

- https://learn.microsoft.com/en-us/windows/apps/publish/get-started
- https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options
- https://learn.microsoft.com/en-us/windows/apps/publish/msstore-dev-cli/overview
- https://learn.microsoft.com/en-us/windows/apps/publish/msstore-dev-cli/publish-command
- https://learn.microsoft.com/en-us/windows/apps/publish/store-readiness-checklist
