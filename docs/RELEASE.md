# Release preparation

Development version: 0.1.0-dev. A public repository has been created at https://github.com/abhisekp/open-download-manager after explicit user approval. Signed source push, release artifacts and workflow execution are recorded separately in the verification report. No Store submission or production infrastructure has been created.

## Prepared workflows

`.github/workflows/ci.yml` builds UI, runs tests/static checks/audit, cross-compiles seven portable targets and uploads CI artifacts. External actions are pinned to verified upstream commit IDs. `.github/workflows/release.yml` is manual and creates a **draft prerelease only**, from the existing `v0.1.0-dev` tag after commit signature verification. Configure `release-approval` as a protected environment with required reviewers before enabling it. Files alone do not establish protection.

The workflows have not run on GitHub. Runner-installed Go/Node tools are inventoried but not pinned; pin reviewed tool versions before reproducibility claims. No SBOM/provenance or executable signing is currently implemented. Checksums establish integrity only, not provenance or publisher authenticity. Complete license inventory, artifact scanning and clean-install tests before release.

## Target matrix

| OS | Architectures | Prepared format | Runtime status |
|---|---|---|---|
| Windows | x86 (386), x64 (amd64), ARM64 | ZIP | x64 local evidence only |
| Linux | x86, x64, ARMv7, ARM64 | tar.gz, Debian .deb | Not runtime-tested |

The Go application includes native BitTorrent and cross-compiles independently of aria2. A browser is required; non-torrent protocols need an architecture-matched aria2 engine. Windows ARM32 is not targeted; ARM means ARM64 on Windows and ARMv7/ARM64 on Linux. AppImage, RPM, Flatpak, Snap and native Windows installers remain packaging milestones rather than fake renamed archives.

## Microsoft Store gate

The manual `store-package.yml` workflow prepares unsigned MSIX review artifacts; Store submission is **not automated or ready**. The current app optionally uses external aria2 for non-torrent protocols and launches a browser; Store feasibility must be validated before packaging. Required next work: decide a compliant native shell/launch model; bundle architecture-matched reviewed engine builds with GPL obligations; reserve an app identity in Partner Center; obtain Publisher/Identity values; create Store artwork and privacy/support pages; generate real MSIX packages; run certification and install/update/uninstall tests; then add authenticated submission automation behind a protected environment. Do not submit an archive as an MSIX or use placeholder publisher identities.

Microsoft supports scripted submissions through its Developer CLI/API and GitHub Actions, but Partner Center identity, role access and certification are separate requirements. Credentials belong in the provider's secure configuration, never chat or source. No Store credential or account was inspected.

## Publication approval

Before any remote write, review the source and report, choose a repository and visibility, and approve the exact signed source push. A new repository should remain private until explicit public-source approval. Open-source intent supplies a licensing direction, but does not publish private workspace contents. Release/tag/Store publication needs its own concrete artifact approval.

Sources checked 2026-11-18/19:
- https://learn.microsoft.com/en-us/windows/apps/publish/
- https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/create-app-submission
- https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/choose-distribution-path
- https://go.dev/doc/install/source
- https://aria2.github.io/manual/en/html/aria2c.html
