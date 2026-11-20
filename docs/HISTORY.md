# Local history and export

Saved terminal records remain bounded to 500 entries and 16 MiB. Newly observed terminal states receive a local recordedAt timestamp; this is when ODM recorded the state, not a claimed server completion time. Legacy records without a timestamp retain unknown time. The cap evicts older recorded states before newer ones instead of sorting random transfer IDs as if they were chronological.

History writes and forget operations roll back their in-memory change if persistence fails. A failed history write does not hide current transfers or disable their controls: status remains available with an explicit warning. Invalid, oversized or non-regular history files are retained for recovery and history customization/export becomes unavailable; ordinary engines can still run. A stale terminal record cannot be used to forget a known pending/running transfer. A forgotten stored result also cannot hide a subsequently observed active row with the same ID.

Settings → Export saved history saves a new JSON file in the configured download folder. It does not upload anything, copy downloaded file contents or read the OS credential store. Default records contain local IDs, states, recorded times, file basenames and lengths. Source links and full paths are excluded. The optional sensitive-details choice includes those paths and URLs, including any private query tokens already present in the saved link. Changing that choice clears approval; the user must approve the displayed local destination again.

Before export, available engine results are refreshed into saved history so a just-completed transfer is not omitted merely because its periodic history flush has not run. If that save fails, the export is refused. If an optional engine cannot be refreshed, the already saved snapshot remains available; it does not claim to include unknown engine state.

Only the owner UI capability can use GET/POST `/api/history-export`; browser submission tokens, wrong origins, unexpected/trailing JSON and missing approval are rejected. One export may run at a time, with a 16 MiB output bound. Names contain a timestamp and random suffix and are created exclusively through the download-directory handle, preventing traversal, leaf-symlink following and replacement of existing files. Success is reported after bytes are flushed and the file closes. On disk failure, an incomplete newly created export may remain; the app reports failure and never removes an unrelated file to clean it up.

This adds export, not arbitrary restore/import or automatic age-based retention. Those require separate format compatibility and user-reviewed deletion policies. Removing history never deletes the downloaded files. Keep optional sensitive exports private.

## Evidence

Tests cover rollback, chronological bounds, active-transfer protection, just-completed export refresh, owner/Origin/consent checks, redacted/sensitive contents, bounded concurrency and retention of an existing user file. The real browser created both export modes; their JSON files were inspected, and the original download SHA-256 remained unchanged. A real app process also completed a transfer while torrent, add-on, history and appearance files were deliberately corrupt; each original component file was retained and failures were reported.
