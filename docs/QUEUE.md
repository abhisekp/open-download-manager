# Native HTTP queue and one-time schedules

Open **Queue & schedules** to add HTTP/HTTPS transfers to the built-in engine, choose a future local start time, reorder pending transfers, or set concurrency. These controls apply to native HTTP downloads even when optional aria2 is installed. Torrent and aria2 queues remain separate.

## Scheduling and recovery

- Save a single future start time within one year. The interface converts the computer's local date/time to a fixed UTC instant; changing the timezone later changes its displayed local time, not the saved instant. The browser resolves ambiguous daylight-saving times according to its local date/time rules.
- No source request is made before the chosen time. The schedule and queue settings must be written successfully before a job becomes runnable.
- ODM must be running. An overdue schedule starts when ODM next runs, including after a crash or sleep. ODM does not wake the operating system, install a background service, or run when it has been closed.
- Pause cancels automatic starting. **Start now** clears the chosen time and queues the transfer immediately. Remove retains existing downloaded files.
- An ordinary queued or actively downloading transfer restores as paused after restart. An unstarted scheduled transfer remains armed. Once a scheduled transfer has started, it follows ordinary interruption/resume rules. Completed schedules do not repeat.
- This is a one-time schedule, not recurring windows or a system power scheduler.

## Ordering and limits

Pending, paused and scheduled transfers can move first/last. Future schedules are skipped until due, so they do not block immediately ready transfers. Active downloads are not preempted by reordering. Rank updates are saved together and rolled back in memory if persistence fails.

Allow 1–8 active downloads with 1–3 per source hostname, no greater than the total active limit. Ports/schemes share the hostname's allowance. Lowering a limit does not cancel active transfers; it prevents additional starts until capacity is available. These are transfer limits, not a security boundary around DNS aliases or redirects. Redirected requests retain the transport's existing limit of three connections per destination host. Native HTTP's shared speed limiter remains separate from other engines.

The queue is bounded to 200 native HTTP records, including terminal records. Forget completed/removed tasks to free slots. Queue order, UTC times and settings are kept in the existing user-private `http-transfers.json` atomic snapshot. Existing development snapshots without the new fields load with default settings (3 active, 3 per host). Older ODM builds that reject unknown snapshot fields cannot downgrade-read a new snapshot; retain an independent data backup before upgrading or downgrading development builds.

## API and verification

`GET /api/http-queue` returns settings and native jobs. `POST` accepts `add` (URL, optional epoch-millisecond `startAt`, and optional `singleStream`), `move` (gid and first/last position), or `settings` (concurrent/perHost). The endpoint uses the existing UI bearer capability, exact Host and Origin protections. Browser submission tokens cannot configure schedules. Input is bounded, rejects unknown/trailing JSON and incompatible action fields, and never accepts embedded URL credentials. No native browser integration permission is expanded. **Use one connection** persists the single-stream choice for that transfer; otherwise suitable large resources can use [native parallel ranges](NATIVE-HTTP.md).

Tests exercise future gating, pause/resume, persisted order, overdue restart, host fairness, concurrency, strict API boundaries and failed-write rollback. `scripts/queue_integration.py` kills and restarts a real disposable process, verifies an overdue transfer's exact bytes and persisted limits, and proves the completed schedule does not repeat. Actual UI checks cover saved settings across reload, local-time scheduling, reorder, pause, Start now and successful download. A loading-state fix prevents transient default settings from appearing before saved settings arrive. Local evidence does not establish the current post-push CI or a published release.
