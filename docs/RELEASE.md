# Release workflows

ODM is an MIT-licensed development application. Packaging success does not establish installation compatibility, certification, executable signing or full feature completion.

## Verification

`ci.yml` runs on pushes, pull requests and manual dispatch. It builds the interface, runs Go race tests and security scans, verifies browser extension tests, builds seven portable targets and four Debian packages, checks package contents and SHA-256 digests, and exercises the real download engines on Linux x64. Node 24.11.1 and the Go toolchain in go.mod are installed explicitly. Actions use immutable upstream commit pins.

Local workflow validation:
```text
actionlint
python -m unittest discover -s scripts -p "test_*.py"
python scripts/verify_artifacts.py --kind portable
```

## Release candidate validation

Run **Prepare draft release** with `validation_only=true` (default) and `source_ref=main` or a signed commit SHA. This verifies the approved commit signer, runs quality gates, builds and inspects all 14 portable/Debian/extension archives, and uploads a `release-candidate` artifact. It creates no tag or release.

To create a draft, first obtain approval for the exact source/version/artifacts and create the reviewed `v0.1.0-dev` tag. Dispatch with `validation_only=false` and that tag. The separate `draft` job requires the configured `release-approval` environment review, verifies that the tag still resolves to the validated SHA, requires successful CI for that exact SHA, downloads the same candidate artifacts and rechecks their digests. It creates an unpublished draft prerelease only. Publishing the draft is a separate approval and operation. Existing drafts fail safely instead of being overwritten.

## Windows package validation

Run **Prepare Microsoft Store package** with `validation_only=true` (default). Windows Server 2025 builds the source and uses the Windows SDK to package x86, x64 and ARM64 unsigned MSIX files with the explicit development identity `OpenDownloadManager.Development`. Archive integrity, manifest architecture, required files and digests are checked. The `development-msix-validation` artifact contains the three MSIX files and their checksums. Development identity packages are not Store submissions.

For reserved-identity packaging, dispatch with `validation_only=false` and the exact Partner Center identity and Publisher. The separate `package` job retains the configured `microsoft-store-review` approval gate. It produces unsigned review files; it does not submit them.

Both protected environments require reviewer `abhisekp` and allow the initiating user to approve, as explicitly requested. Validation jobs have read-only repository permission and no environment secrets.

## Remaining release work

Public GitHub prerelease publication is now an optional, separately approved job, selected with publish_after_review=true outside validation mode. No public binary release or Store submission has been executed. Store identity/account access, certification, signing and clean install/update/uninstall tests remain necessary. Release candidates now generate a CycloneDX 1.6 dependency/artifact inventory, validate it against pinned official schemas, and generate GitHub/Sigstore provenance and SBOM attestations. These records are not executable Authenticode signatures. Windows x64 and Linux x64 have engine test evidence; cross-compilation alone does not verify the other architectures. ZIP, tar.gz, Debian and unsigned MSIX formats are implemented; RPM, AppImage, Flatpak and Snap remain planned.

Keep secrets out of source and chat. Retain previous packages for a reviewed manual reinstall if necessary; no automatic rollback or credential rotation is performed.

## Additional release gates

CI now includes native ARM64 Windows/Linux tests, Linux x86 startup, emulated Linux ARMv7 startup, and Debian x64 install/transfer/reinstall/uninstall checks. Store validation includes Windows x86 startup and an x64 MSIX install/launch/version-upgrade/uninstall test on an isolated GitHub runner. Its ephemeral test certificate is trusted only within the disposable runner; it is never a release signing identity and signed test copies are not uploaded. Configuring these checks does not establish that they pass; inspect the current run results.

Release attestations require the checked-out source to equal the workflow dispatch SHA. Select the intended branch/tag as the workflow ref; draft creation should be dispatched on the reviewed tag. The source_ref input must resolve to the same SHA. Provenance verification enforces repository/workflow identity, artifact digest and exact source revision.

Microsoft signs MSIX packages submitted through the Store after certification. A separately purchased signing certificate is not required for that route. Independently distributed Windows executables/MSIX have separate signing requirements. Partner Center account/name reservation, listing and app identity are still required. Source: https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options .

## Prepared publication operation

The optional publish job re-downloads the uploaded draft assets, verifies package digests and the source-bound attestations, then publishes the prerelease only after a second release-approval environment review. Validation mode never creates tags, drafts or releases. Store publication remains blocked by owner account registration and first-submission prerequisites; see STORE-READINESS.md.
