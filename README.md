# Open Download Manager

ODM targets an independent, MIT-licensed desktop download manager combining IDM, FDM and MDM-style functionality with a first-class extension/add-on platform. See the [product brief](docs/PRODUCT-BRIEF.md).

The current build is an early implementation based on the selected light/blue design. The portable service opens a local browser interface; a [native desktop window](docs/DESKTOP.md) is being validated separately. **Combined feature parity, the full extension platform and production distribution remain incomplete.**

## Run

Install Go 1.26.11 and Node.js 22+ for source builds. HTTP/HTTPS downloads, native parallel ranges and native torrents work without aria2. For FTP/SFTP or the optional aria2 transfer engine, install aria2 for your operating system and CPU and put `aria2c` on PATH. The application never installs or downloads an engine automatically.

```sh
npm ci --prefix ui --ignore-scripts
npm run build --prefix ui
go build -o odm .
./odm
```

Windows: build with `go build -o odm.exe .` and run `odm.exe`. It opens a browser to an authenticated loopback session. `--no-browser` prints the launch link; treat that link as a local credential and do not share it. `--engine` accepts a full aria2 executable path. `--downloads` selects the download directory. `--data-dir` selects application data. Stop with Ctrl+C to save unfinished transfers; closing the browser leaves the app running. OS-backed data locking prevents concurrent ownership and recovers after abrupt termination. Legacy releases without an ownership marker are rejected conservatively; stop the old release before clearing its legacy lock.

Portable archives under `dist/` contain the application, not aria2. HTTP/HTTPS and torrents are built in. Portable browser-interface builds need a browser (`xdg-open` on Linux), or the launch link can be opened manually. Separate native window/tray candidates are built for Windows, Linux and macOS x64/ARM64; native installers and updates remain pending.

## Current behavior

- Download BitTorrent magnets and import `.torrent` files using the in-process MIT-licensed Rain library. Native torrent state is saved in a separate local database.
- Add HTTP/HTTPS file URLs through the built-in engine when aria2 is absent, or explicitly through Queue & schedules. Native HTTP supports three parallel ranges for suitable files of at least 8 MiB, validated per-range crash checkpoints and a single-connection option. It includes bounded queues, safe collisions, TLS checks, stall cancellation and persisted speed limits. Installed aria2 separately handles its HTTP and FTP/SFTP transfers. See [native HTTP behavior](docs/NATIVE-HTTP.md).
- View progress, speed, connections and transfer states; pause, resume, remove, search, and filter torrents.
- Create persistent category labels, assign them to downloads and filter by category. Category changes never move or delete files; see [categories](docs/CATEGORIES.md).
- Import up to 50 newline-separated links with duplicate removal and per-link results.
- Verify the first file of a completed transfer against an expected SHA-256 checksum (up to 1 GiB).
- Preserve up to 500 terminal transfer records locally and export reviewed JSON history with sensitive source links and paths excluded by default. See [history](docs/HISTORY.md).
- Persist separate native torrent download/upload caps and a native HTTP/optional aria2 download limit. Existing torrent peers and webseeds observe live changes; each engine enforces its own budget. See [torrent bandwidth](docs/TORRENT-BANDWIDTH.md).
- Save unfinished transfer sessions every ten seconds and on graceful shutdown.
- Use **Queue & schedules** for persisted native HTTP order, 1–8 active / 1–3 per-host limits and one-time local-time starts. Unstarted schedules remain armed across restarts; paused schedules do not fire. ODM must be running; see [queue behavior](docs/QUEUE.md).
- Pair a browser extension with a dedicated, revocable link-submission token. See [browser integration](docs/BROWSERS.md) for packages and current marketplace limitations.
- Fetch/copy a live tracker list, then separately review and add chosen URLs to a public native torrent. Private torrents and magnets awaiting metadata cannot receive additions. Reviewed paused tracker replacement/removal is atomic and rejects stale lists; see [native torrent controls](docs/NATIVE-TORRENTS.md).
- Install signed local URL-resolver add-ons, review exact host permissions, enable/disable them, and review suggested links before downloading. Modules run in isolated WASI workers. API 2 provider HTTPS requests require separate per-link/exact-package approval; API 1 retains no network access. See the [SDK](sdk/README.md) for the contract and remaining limits.
- Enable optional native desktop completion/failure notifications from Settings. Notices hide file names and links. See [notification behavior and OS limitations](docs/NOTIFICATIONS.md).
- Keep downloaded files when removing a transfer. Never automatically execute downloads.

HTTP and native BitTorrent transfers have local end-to-end evidence. FTP, SFTP, public swarm downloads and external authenticated servers have not been integration-tested. Completed history is stored locally in a bounded JSON file and survives graceful restart. An interrupted write preserves the prior file; a crash before the five-second history snapshot can lose the newest terminal entries. Forgetting history does not delete downloaded files. Sessions store source URLs; keep application data private. DHT/PEX are disabled and downloads stop on completion by default. [Approved bounded seeding sessions](docs/SEEDING.md) can share complete files until a persisted deadline. Native torrent peer connections can use the network; no firewall rule is installed automatically. Private torrent metadata must never be rewritten to bypass tracker restrictions.

## Verification

```sh
go test ./...
go vet ./...
npm run build --prefix ui
npm run test:sites --prefix ui
npm audit --prefix ui
python scripts/build.py
```

For the real transfer test, launch a separate test instance with `--no-browser --data-dir <test-data> --downloads <test-downloads>` and redirect its standard output to a launch log. Run `python scripts/integration.py --launch-log <log> --downloads <test-downloads>`. The test uses a local HTTP fixture, checks pause/resume and SHA-256 integrity, and never downloads third-party files. Do not use a personal download session for this test.

## Architecture

Go embeds the production React interface and exposes a narrow API bound to 127.0.0.1. Each launch generates independent UI and engine tokens. API calls require a bearer token, exact Host and same-origin checks. No CORS is enabled. CSP denies remote scripts and framing. BitTorrent runs in-process through Rain; its database and data directory are local. The optional aria2 process has its own temporary configuration and RPC token; the application never attaches to an existing user engine.

Only the local authenticated user may add URLs. Downloads deliberately allow local-network URLs; this is not safe to expose as a remote multi-user URL-fetching service. No shell commands or arbitrary engine RPC methods are exposed by the UI API. Engine configuration uses file permissions on Unix; Windows inherits the selected directory ACL. Keep the data directory user-private.

HTTPS Basic website accounts can be saved in the device credential store and selected explicitly in Queue & schedules. See [saved logins](docs/SAVED-LOGINS.md) for scope, limits, recovery and platform requirements.

See [ROADMAP](docs/ROADMAP.md), [release plan](docs/RELEASE.md), [extension contract](docs/EXTENSIONS.md), [MCP plan](docs/MCP.md), and [verification report](docs/VERIFICATION.md).

## License

New application code is MIT licensed. Rain, React and Bootstrap Icons are MIT licensed; preserve their license notices when redistributing. aria2 is separately installed GPL software and is not included in these artifacts. Future bundled engines require license/source-distribution review. FDM code, binaries, branding and assets were not copied.
