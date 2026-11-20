# Saved HTTPS website logins

Settings → Saved website logins stores a new username/password for one HTTPS origin in the operating-system credential store. Queue & schedules offers only ready logins for the entered website. Each download requires an explicit selection; ordinary downloads and browser companions do not receive automatic access to saved accounts.

Supported authentication is HTTP Basic over verified TLS. Windows uses Credential Manager, macOS uses Keychain, and Linux uses an unlocked Secret Service provider. Linux installations without a working desktop keyring cannot save logins; there is no plaintext fallback. FTP/SFTP credentials, browser cookies, OAuth and integrated Windows sign-in remain separate, unimplemented account features.

## Scope and recovery

- Scope is an exact normalized HTTPS hostname and port. A subdomain or different port is another origin. Password headers are removed on cross-origin redirects, including parallel range requests. HTTPS-to-HTTP redirects remain rejected.
- Passwords and usernames are not written to application JSON, transfer URLs, logs or API responses. Application data contains labels, website origins, random login references and operation state. Labels and website names are visible metadata: do not put passwords in labels.
- At most 30 references are retained. Usernames are limited to 128 UTF-8 bytes without colons/control characters; passwords to 768 UTF-8 bytes without control characters. The serialized secret has a smaller combined 1,400-byte limit for platform compatibility. A different label is required for another account on the same website.
- Passwords are necessarily present in process memory during authentication; the bounded cache expires after five minutes and is cleared at shutdown. This is not a promise of memory zeroization or protection against a compromised OS account.
- Save and removal journal their intent before accessing the OS store. An interrupted save can be reconciled with Check; interrupted removal must be retried as removal. Only exact recorded keys are accessed; the app never enumerates or deletes unrelated OS credentials. Removing application data alone does not remove previously saved OS keys: remove saved logins in Settings first.
- One OS operation is allowed at a time. Download lookups stop waiting after ten seconds or cancellation. An OS prompt/helper already in progress can remain until the operating system finishes it; further operations do not spawn more helpers. Complete the local unlock prompt, then Check or retry. App shutdown prevents late results from writing to a released data directory.
- Pause a failed download, choose Change login, select a ready replacement (or No saved login), and approve restarting its partial data. The transfer stays paused and its checkpoints/validators are reset. Start now begins a fresh download. Accounts can return different content under the same ETag, so reusing the old bytes would be unsafe.
- Removing a login clears its cache and blocks later lookups. Already authorized HTTP connections may finish. Pause affected downloads first. Backups contain references, not transferable passwords; a restore on another device may require replacement logins.

The local owner API exposes `GET/POST /api/logins` and an approved `login` action on `/api/http-queue`. Existing bearer, Host and Origin protections apply; the browser submission token cannot read or edit accounts. Neither endpoint returns passwords or usernames. Input is bounded and rejects unknown/trailing JSON.

## Verification

Go tests use real trusted local TLS fixtures for authenticated single/parallel transfers, cross-origin header stripping, changed-account partial reset, no-secret registry content, corrupted references, operation contention, interrupted writes, and shutdown with a late OS result. API tests cover consent, malformed input, owner/browser-token separation and hostile origins. No real customer account is used.

`python scripts/os_keyring_check.py` explicitly writes, reads, removes and verifies absence of one random, owned, public dummy credential. It does not inspect existing entries. Windows can run it locally. Linux creates a private D-Bus session and temporary Secret Service data directories (requires `dbus-run-session`, `dbus-send` and `gnome-keyring-daemon`). macOS setup is restricted to disposable CI runners; it creates an isolated test keychain, restores the previous keychain settings and deletes its own fixture keychain. A passing simulated backend test is not evidence of a working OS store.

Windows local OS lifecycle and the actual Chrome UI save/select/schedule/pause/change/remove journey passed on 2026-11-20 with dummy credentials. The download itself was scheduled for the future and cancelled, so the UI test did not contact an external website. Linux and both macOS keyring checks require fresh workflow acceptance for this change.
