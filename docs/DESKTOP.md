# Native desktop window

The Electron shell uses ODM's existing Go service and embedded interface. It adds a native window, single-instance activation, application/edit/view menus, zoom, tray show/hide and explicit quit. Closing the window quits and saves downloads; **Hide window (keep downloading)** in the tray is the explicit background action. A missing tray service does not change close behavior.

The shell starts only its packaged backend; command-line URLs or arbitrary arguments are not forwarded. It accepts only explicit `--data-dir` and `--downloads` absolute paths for backend configuration. A custom data directory gets a separate desktop profile and instance lock. The random local address and credential arrive through a private child-process pipe, never an external browser command line or log. The renderer has no Node integration, preload bridge or filesystem API. Context isolation, Chromium sandbox and web security stay enabled. It may load resources only from the exact backend origin; external navigation, popups, downloads and permission grants are denied. The Go API retains its existing bearer, Host and Origin protections. This local HTTP connection is confined to loopback; external download traffic is performed by the backend.

Data stays in the same `OpenDownloadManager` configuration directory as the portable backend. Downloads use the operating system's Downloads directory. Quit requests the authenticated shutdown API and waits up to ten seconds before forced termination. An unexpected backend exit closes the shell with an error. Existing backend lock/checkpoint recovery applies on the next launch. Starting the shell while a standalone backend owns the same profile is rejected; it cannot attach to an unknown process.

## Build and exercise

Use the Go/Node versions in CI and build the UI first:

```text
npm ci --prefix ui --ignore-scripts
npm run build --prefix ui
python scripts/prepare_desktop.py
cd desktop
npm ci --ignore-scripts
node node_modules/electron/install.js
npm test
npm run smoke
node package.cjs
node verify-package.cjs
node packaged-smoke.cjs
```

Electron's reviewed installer downloads its pinned upstream runtime and verifies bundled checksums. Linux CI configures the runtime's setuid sandbox helper and runs the native window under Xvfb; it does not disable Chromium sandboxing. Packaged Linux distributions still need a correctly installed sandbox helper or a working supported user-namespace configuration. Test on the intended Linux distribution before distribution.

Linux packages use a directory name without spaces. The actual-runtime test found Chromium's [setuid launcher path bug](https://github.com/electron/electron/issues/44414): a space in the installation path truncates its child executable path and aborts startup. Extract Linux desktop candidates under a parent path without spaces. This is an upstream runtime limitation; sandboxing is not disabled to work around it. Installers should use a fixed safe path such as `/opt/open-download-manager`.

The separate desktop workflow builds and exercises six native targets: Windows x64/ARM64, Linux x64/ARM64 and macOS Intel/Apple Silicon. The current upstream Electron runtime does not provide Windows x86, Linux x86 or ARMv7 builds. Those three targets retain the existing portable Go service/browser interface. The nine portable targets remain independently built and tested.

## Distribution boundaries

Native candidates are development artifacts, not published releases or signed installers. They do not yet replace the portable/MSIX release pipeline. Tests execute source and packaged application code with the real backend, and a separate external driver launches the actual renamed executable with disposable directories. That test enables a temporary loopback debugging connection only on its own invocation, checks the actual window, real transfer bytes, saved state and clean exit, then removes its temporary profile. OS installer integration remains separate. Electron/Chromium runtime licenses are retained by the packager alongside the Go and application notices. Add the actual runtime components to release SBOM/provenance and verify installation and updates before publishing a desktop release. Keep pinned runtime updates and security review part of maintenance.

Preparation records `ODM-BUILD.json`: source revision, whether the working tree is modified, a digest of source inputs plus the actual ignored UI build outputs embedded by Go, and SHA-256 hashes of the shipped backend/resources. Packaging refuses a changed source tree, changed embedded UI or replaced prepared backend even when VERSION is identical. The package verifier reads back every recorded resource hash. Candidate metadata derives its source from that preparation record and preserves `sourceDirty`; a local modified build cannot be labeled as a clean reviewed source build. These records are integrity checks, not publisher signatures, remote attestations, a complete runtime SBOM or protection against a malicious build machine.

Current runtime/security references: [Electron upstream releases](https://github.com/electron/electron/releases), [Electron security guidance](https://www.electronjs.org/docs/latest/tutorial/security), [packager](https://github.com/electron/packager). Architecture availability was checked against Electron 45.4.1 release assets.

macOS is direct download only. Apple Store enrollment, paid Apple Developer membership and Apple notarization are excluded by the owner; do not represent local/ad-hoc bundle signatures as an identified Apple publisher. Windows production publisher signing and Microsoft Store certification remain separate gates.
## Packaged runtime restrictions

Native packages disable Electron's Node-as-executable mode, NODE_OPTIONS/NODE_EXTRA_CA_CERTS startup overrides, Node inspector arguments and extra file-protocol privileges. They load application code only from app.asar. Embedded ASAR validation is enabled on Windows/macOS, where Electron supports it; Linux retains the explicit lack of that validation. The build requires an explicit setting for every supported fuse and reads the actual executable back before accepting the package.

The source smoke test uses the ordinary development Electron runtime. The separate packaged-executable test sets a harmless owned NODE_OPTIONS preload and ELECTRON_RUN_AS_NODE, then verifies the hardened application still starts as an app and never writes the preload's marker. It also checks isolated rendering, exact transfer bytes, saved state and clean exit. The hardening helper and all other build/test code are excluded from the four-file application archive.

Cookie encryption remains off because this application uses memory-only renderer sessions, not persistent browser cookie storage. The optional custom V8 snapshot is not enabled because it is not in the prebuilt runtime. macOS package-time ad-hoc signature repair permits runtime testing; it is not identified-publisher signing, notarization or Apple Store distribution. Fuses must be applied before any future publisher signature. Binary flags and a sidecar JSON record do not substitute for signed releases, runtime dependency inventory or a trusted updater.

Implementation uses pinned build-only @electron/fuses 2.1.3. Official references: [Electron fuses](https://www.electronjs.org/docs/latest/tutorial/fuses) and [the upstream tool](https://github.com/electron/fuses), checked 2026-11-20.
