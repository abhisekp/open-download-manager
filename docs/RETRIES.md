# Automatic native HTTP retries

New native HTTP/HTTPS downloads in the application receive three automatic retries after the initial attempt. Queue & schedules offers an explicit 0–5 retry choice; zero requires manual retry. Existing stored jobs keep their original policy, so an old paused/failed download is not silently enabled for background requests. The optional aria2 and torrent engines have separate policies.

Eligible failures are temporary connection/read interruptions, stalled bodies and HTTP 408, 429, 500, 502, 503 or 504. Retries use exponential backoff (2, 4, 8 seconds for the default budget), with up to 25% additional jitter. A valid Retry-After delay or HTTP date sets a minimum wait. Requests for more than 15 minutes stop for manual action instead of shortening the server's wait. Invalid Retry-After values use ordinary backoff. DNS not-found, invalid TLS certificates, blocked redirects, authentication refusals, invalid ranges/validators, oversized bodies and local disk/checkpoint errors are not automatically repaired.

Attempt counts and the next due time are persisted before a retry becomes runnable. Waiting retries release their active/per-host queue slot. Resuming uses only synced offsets and existing resource validators; a changed source restarts through the established isolation path. Parallel retries reuse only validated range checkpoints. Neither a retry nor a source-change fallback can finalize an incomplete response.

The queue shows Waiting to retry, the next start time, the last failure and the used retry budget. Pause cancels the pending retry. Start now explicitly begins a new attempt cycle and resets its retry count, including a manual override of a server delay. Removing/forgetting a waiting job prevents its retry. Changing saved accounts resets retry state together with partial progress. Exhaustion leaves a failed download and its partial data for review.

A pending retry is an explicit persisted start intent: closing/crashing the app does not discard it. It runs when ODM next opens after the due time. An interrupted active request restores paused as before. A completed request is not repeated after restart. No OS wake, background service or application-wide torrent/aria2 retry policy is implied.

## Verification

Real local HTTP/TLS tests cover bounded attempts, permanent refusals, TLS/redirect classification, valid/very-long Retry-After values, zero-retry choice, queue fairness, partial-body recovery with If-Range, parallel-range recovery, failed intent writes and pause cancellation. A separate real-process test kills ODM with a retry pending, restarts after its deadline, verifies exact downloaded bytes and attempt count, then checks pause/restart cancellation and no repetition of a completed request. That process check is included in Linux and native Windows/macOS CI jobs.

The actual Chrome UI recovered a temporary 503 with a matching file SHA-256, honored zero retries and paused another delayed retry. Fixture request counts were respectively 2, 1 and 1. These are local fixture results; broader origin/CDN/proxy behavior remains part of production acceptance.

Retry-After parsing follows the delay/date forms in [RFC 9110 section 10.2.3](https://www.rfc-editor.org/rfc/rfc9110.html#name-retry-after). The 15-minute limit and retry budget are ODM product policy.
