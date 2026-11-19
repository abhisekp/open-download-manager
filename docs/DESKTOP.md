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

The separate desktop workflow builds and exercises six native targets: Windows x64/ARM64, Linux x64/ARM64 and macOS Intel/Apple Silicon. The current upstream Electron runtime does not provide Windows x86, Linux x86 or ARMv7 builds. Those three targets retain the existing portable Go service/browser interface. The nine portable targets remain independently built and tested.

## Distribution boundaries

Native candidates are development artifacts, not published releases or signed installers. They do not yet replace the portable/MSIX release pipeline. Tests execute source and packaged application code with the real backend, and a separate external driver launches the actual renamed executable with disposable directories. That test enables a temporary loopback debugging connection only on its own invocation, checks the actual window, real transfer bytes, saved state and clean exit, then removes its temporary profile. OS installer integration remains separate. Electron/Chromium runtime licenses are retained by the packager alongside the Go and application notices. Add the actual runtime components to release SBOM/provenance and verify installation and updates before publishing a desktop release. Keep pinned runtime updates and security review part of maintenance.

Current runtime/security references: [Electron upstream releases](https://github.com/electron/electron/releases), [Electron security guidance](https://www.electronjs.org/docs/latest/tutorial/security), [packager](https://github.com/electron/packager). Architecture availability was checked against Electron 45.4.1 release assets.

macOS is direct download only. Apple Store enrollment, paid Apple Developer membership and Apple notarization are excluded by the owner; do not represent local/ad-hoc bundle signatures as an identified Apple publisher. Windows production publisher signing and Microsoft Store certification remain separate gates.
