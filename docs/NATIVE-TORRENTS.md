# Native BitTorrent

ODM integrates Rain v2.5.1 in its own process. Magnet links and HTTP(S) URLs ending in `.torrent` route to this engine; the Add dialog also accepts a local `.torrent` metadata file. Torrent transfers do not use aria2 and work when aria2 is absent. Other protocols still need the optional aria2 engine.

State lives in `torrents.db` under the selected application data directory. Each torrent downloads into its own subdirectory below the selected download folder. Metadata imports are limited to 4 MiB and unsafe path components are rejected; magnet metadata also uses Rain's storage/path validation. DHT and peer exchange are disabled. The engine listens for torrent peers, so normal OS firewall/network policy applies. No automatic firewall rule is installed. Torrents stop after download completion; seeding controls, file selection/priorities, and native bandwidth-limit controls remain work in progress.

Rain reports active byte totals as zero after stopping even when its persisted verified-piece bitfield is complete. ODM normalizes a complete verified-piece count to total bytes. Partial paused-progress counters may be less detailed than active counters. No completed state is inferred from downloaded network bytes alone.

Native tests construct original fixture metadata and run a seed and download client on loopback. They verify a real peer transfer, metadata exchange from a magnet, file-byte integrity, persistence after closing/reopening the native session, and rejection of unsafe imported metadata. No public swarm was contacted. `scripts/native_mode_test.py` verifies application startup and torrent import with a deliberately missing aria2 path. Public tracker compatibility, BitTorrent v2/hybrid metadata, private torrent behavior and platform-specific firewall UX remain unverified.

Rain is MIT licensed. One old transitive helper module lacked a license text in its pinned source. ODM replaces the narrow required API surface with original MIT code in `internal/nettoolscompat`: compact IPv4/IPv6 peer address encoding and bounded per-host throttling. This replacement is tested separately; it does not claim to implement the upstream helper's unused tunnel API. DHT remains disabled. Dependency license notices are generated from actual build dependencies with `scripts/notices.py`.

The UI and authenticated API maintain a unified transfer list, but native torrent lifecycle and persistence are separate from HTTP transfers. A saved torrent database is not an aria2 session file, and neither should be edited while the application is running.
