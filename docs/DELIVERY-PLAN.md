# Remaining production work

The full independent IDM/FDM/MDM scope in PRODUCT-BRIEF.md remains the objective. This is a delivery plan, not a completion claim. Existing native engines, approved design/icon and MIT application license remain in place. No hosted account service is needed for ordinary downloads.

## Ordered milestones

1. **Queue and scheduling:** persisted native HTTP queue order and concurrency/per-host limits, explicit one-time UTC schedules with local-time UI, pause/cancel/resume and missed-start recovery. New API retains existing bearer/Host/Origin guards and bounded strict input. A schedule is persisted before it can start. Test real requests, future start gating, overdue restart, cancellation and storage failures. No database or external-service changes. Torrent/optional-engine queues, recurring windows and sleep/wake OS integration need later acceptance.
2. **Transfer core:** native segmentation and validator-safe recovery, authenticated credentials in OS secure stores, retries and real FTP/SFTP/auth-server matrix. Never persist plaintext credentials in transfer URLs.
3. **Torrent control:** file selection/priorities, transfer/seeding limits, tracker removal/replacement with private-torrent safeguards, and compatibility/swarm verification.
4. **Extension platform:** bounded network/credential APIs, events and UI contributions, compatibility/update recovery, extra signed examples and hard process resource containment.
5. **Desktop and organization:** categories/destinations, safe file operations, retention/export, accessibility/keyboard/notifications, native installer and update/uninstall journeys.
6. **Advanced providers:** original controlled media/collection/site providers with safe helper lifecycle, previews/conversion and scoped automation APIs. Never describe generic UI or a resolver contract as an implemented provider.
7. **Distribution:** integrate native desktop artifacts with inventories/provenance and reviewed release gates; verify installed upgrades/rollback; complete Store identity and certification. Browser marketplace certification/public installation remains independently verifiable. Chrome registration is payment-blocked; Apple Store/paid membership are excluded.

Each milestone requires implementation, meaningful success/failure tests, actual UI/API evidence, and updated acceptance notes. Work proceeds locally within authorization. Public packages/releases and consequential external changes retain their existing approval requirements. Production readiness requires all required gates to pass for one exact candidate revision, not a combination of unrelated older runs.
