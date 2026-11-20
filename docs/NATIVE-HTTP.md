# Native HTTP transfers

HTTP/HTTPS works without a separate engine. Native transfers use bounded concurrency, user-private persisted state, safe destination names, crash checkpoints, shared rate limiting, certificate verification, bounded headers/redirects, stalled-body cancellation and journaled finalization. Existing downloads are never executed automatically.

## Parallel ranges

A new full download of at least 8 MiB can use three fixed parallel byte ranges when the response advertises byte ranges and supplies a strong ETag. Smaller resources, weak/missing validators, unadvertised range support and existing single-stream partial downloads retain the sequential path. **Use one connection** in Queue & schedules explicitly disables parallel ranges for that transfer and persists the choice. No fixed speed multiplier is promised; this is not dynamic 32-connection splitting.

The initial response is closed before range workers start. Each range uses the exact strong validator in If-Range and must return matching ETag, exact Content-Range and an appropriate length. Chunked range bodies are allowed but bounded to their requested extent. Unsupported range responses or changed validators discard the old layout and restart once through the single-stream path. Malformed, short, overlong or stalled bodies fail without publishing a completed destination file. A retry never silently combines content from different declared versions.

Range writes are disjoint and use the existing owned regular partial file under `os.Root`. Sparse file size is never evidence of coverage. Per-range offsets are saved only after file sync. A full-range checkpoint also requires a clean response ending, so an overlong response cannot save a complete range and bypass validation on resume. Crash tails beyond the declared resource are truncated; all uncommitted tails are overwritten when ranges resume. Checkpoints are validated for contiguous, non-overlapping coverage, sizes, strong validator and aggregate progress when loaded. Invalid registry bytes are retained for recovery rather than overwritten.

The HTTP transport retains its three-connections-per-destination cap. All workers share the native HTTP speed limiter, rather than receiving separate budgets. Queue limits count transfers; parallel range connections are not additional queue entries. Actual native worker connection counts are exposed in transfer status. Optional aria2 still handles its own transfers and FTP/SFTP when installed; neither native nor aria2 HTTP traffic is a shared application-wide budget with native torrents.

## Recovery and limitations

Active transfers restore paused. Explicit Resume continues only synced, validated offsets. Source changes can require a full restart. Errors require user retry; bounded automatic backoff, authentication beyond the explicit [HTTPS Basic saved-login flow](SAVED-LOGINS.md), mirror downloads, dynamic re-splitting and a wider origin/CDN compatibility matrix remain work. Basic validators do not replace an expected cryptographic checksum from a trusted source.

Segment layout and the single-connection preference extend the existing development registry. Older builds that reject unknown fields cannot downgrade-read it. Keep an independent data backup when testing development upgrades/downgrades. The public v0.1.0-dev release does not contain this source.

## Evidence

Real local HTTP tests cover parallel requests, exact bytes, pause/restart, persisted range tails, changed content, ignored ranges, weak validators, chunked/short/overlong/malformed/stalled responses, shared throttling, concurrent pause, snapshot independence and malformed checkpoint preservation. The process test kills a real app after a durable range checkpoint, resumes the same resource, and separately replaces the resource before resuming; both paths verify exact SHA-256 bytes. Actual browser UI testing covers the three-range path and single-connection option. Platform CI outcomes must be read separately for the exact signed source. Cross-compilation alone is not runtime or installation evidence.
