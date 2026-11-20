# Native torrent bandwidth

In Settings, choose separate native torrent download and upload caps in whole KiB/s. Zero means unlimited; the supported range is 0–1048576. These are session-wide budgets shared by all native peers and webseeds, separate from native HTTP and optional FTP/SFTP engine limits. Torrents still stop on completion; this feature does not enable indefinite seeding.

Changes apply to existing peers without reconnecting. Pending reservations wake on a change or shutdown. Saving the same values preserves the current budget instead of replenishing its burst. The limiter uses the existing [Go rate package](https://pkg.go.dev/golang.org/x/time/rate) with bounded 16–64 KiB bursts; buffered network data, protocol overhead and already permitted payload mean this is not an exact network-interface byte ceiling. Download caps govern consumed torrent payload; operating-system socket buffers can receive ahead. Deliberate webseed bandwidth waits do not consume the network read timeout.

The owner-only `GET/POST /api/torrent-limits` API uses the existing bearer, Host and Origin checks. A POST requires both `downloadKiB` and `uploadKiB`, rejects unsupported, fractional, unknown and trailing values, and is limited to 1 KiB. Browser pairing tokens and add-ons do not gain access.

Settings are stored as version 1 in `torrent-limits.json` in the app data directory. A flushed temporary file replaces the settings before the active session changes; a failed save leaves previous limits active. Invalid or oversized state is preserved and the native torrent component reports unavailable, while ordinary HTTP downloads remain usable. No fallback silently overwrites the owner's original settings.

Local tests exercise real capped download and upload peers, unchanged output bytes, changing an existing slow peer to unlimited, shared-worker budgets, cancellation, a real HTTP webseed whose bandwidth wait exceeds its read timeout, API refusal, failed-write rollback and restart persistence. These tests use original loopback fixtures. Public swarm, router and firewall behavior remains a separate acceptance gate.
