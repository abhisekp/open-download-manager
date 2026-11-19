# Open Download Manager

ODM targets an independent, MIT-licensed desktop download manager combining IDM, FDM and MDM-style functionality with a first-class extension/add-on platform. See the [product brief](docs/PRODUCT-BRIEF.md).

The current build is an early implementation based on the selected light/blue design. The portable service opens a local browser interface; a [native desktop window](docs/DESKTOP.md) is being validated separately. **Combined feature parity, the full extension platform and production distribution remain incomplete.**

## Run

Install Go 1.26.11 and Node.js 22+ for source builds. HTTP/HTTPS downloads and native torrents work without aria2. For FTP/SFTP and accelerated parallel file transfers, optionally install aria2 for your operating system and CPU and put `aria2c` on PATH. The application never installs or downloads an engine automatically.

```sh
npm ci --prefix ui --ignore-scripts
npm run build --prefix ui
go build -o odm .
./odm
```

Windows: build with `go build -o odm.exe .` and run `odm.exe`. It opens a browser to an authenticated loopback session. `--no-browser` prints the launch link; treat that link as a local credential and do not share it. `--engine` accepts a full aria2 executable path. `--downloads` selects the download directory. `--data-dir` selects application data. Stop with Ctrl+C to save unfinished transfers; closing the browser leaves the app running. OS-backed data locking prevents concurrent ownership and recovers after abrupt termination. Legacy releases without an ownership marker are rejected conservatively; stop the old release before clearing its legacy lock.

Portable archives under `dist/` contain the application, not aria2. The basic HTTP/HTTPS and torrent engines are built in; optional engine installation adds other protocols and accelerated transfer features. Linux needs a browser and `xdg-open`, or open the printed link manually. There is no desktop shortcut/tray/native window yet.

## Current behavior

- Download BitTorrent magnets and import `.torrent` files using the in-process MIT-licensed Rain library. Native torrent state is saved in a separate local database.
- Add HTTP/HTTPS file URLs through the built-in fallback when aria2 is absent; installed aria2 handles accelerated HTTP and FTP/SFTP. Native HTTP includes a three-transfer queue, validator-aware resume, crash checkpoints, safe collision handling, TLS checks, stall cancellation and persisted per-engine speed limits.
- View progress, speed, connections and transfer states; pause, resume, remove, search, and filter torrents.
- Import up to 50 newline-separated links with duplicate removal and per-link results.
- Verify the first file of a completed transfer against an expected SHA-256 checksum (up to 1 GiB).
- Preserve up to 500 terminal transfer records locally across graceful restarts.
- Persist a download speed limit for native HTTP and the optional aria2 engine; native torrent limits are not exposed yet. Each engine enforces its own limit rather than a shared application-wide budget.
- Save unfinished transfer sessions every ten seconds and on graceful shutdown.
- Pair a browser extension with a dedicated, revocable link-submission token. See [browser integration](docs/BROWSERS.md) for packages and current marketplace limitations.
- Fetch/copy a live tracker list using the built-in, permission-gated tracker-feed capability.
- Install signed local URL-resolver add-ons, review exact host permissions, enable/disable them, and review suggested links before downloading. Modules run in isolated WASI workers; see the [SDK](sdk/README.md) for the contract and remaining limits.
- Keep downloaded files when removing a transfer. Never automatically execute downloads.

HTTP and native BitTorrent transfers have local end-to-end evidence. FTP, SFTP, public swarm downloads and external authenticated servers have not been integration-tested. Completed history is stored locally in a bounded JSON file and survives graceful restart. An interrupted write preserves the prior file; a crash before the five-second history snapshot can lose the newest terminal entries. Forgetting history does not delete downloaded files. Sessions store source URLs; keep application data private. DHT/PEX are disabled and seeding stops on completion. Native torrent peer connections can use the network; no firewall rule is installed automatically. Private torrent metadata must never be rewritten to bypass tracker restrictions.

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

See [ROADMAP](docs/ROADMAP.md), [release plan](docs/RELEASE.md), [extension contract](docs/EXTENSIONS.md), [MCP plan](docs/MCP.md), and [verification report](docs/VERIFICATION.md).

## License

New application code is MIT licensed. Rain, React and Bootstrap Icons are MIT licensed; preserve their license notices when redistributing. aria2 is separately installed GPL software and is not included in these artifacts. Future bundled engines require license/source-distribution review. FDM code, binaries, branding and assets were not copied.
