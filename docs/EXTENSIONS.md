# Extension API v1 (development)

The product target is a first-class application extension platform, described in [PRODUCT-BRIEF.md](PRODUCT-BRIEF.md). It is separate from browser companion extensions. The implementation below is only the current built-in capability and does not satisfy the full add-on requirement. SDK/API design and execution isolation are early architecture milestones, rather than work deferred until every advanced integration is hard-coded.

The current executable registers one declarative manifest in `extensions.go`. It has ID, name, version, capability, endpoint and description. The only capability is `tracker-feed`; the only approved endpoint is `https://newtrackon.com/api/live`. Host code performs HTTPS access with a timeout, rejects redirects, bounds the feed to 256 KiB / 500 unique entries, and validates protocols.

The permission checkbox is per page session. The host API also requires an explicit enabled grant on each fetch. It does not grant arbitrary domains, filesystem access, scripts, subprocesses or generic RPC. Revocation disables subsequent requests; it does not cancel a request already in flight.

This is an extension foundation, not an installable ecosystem. Third-party loading, a marketplace, package signing, persisted grants, extension disable/upgrade lifecycle and public SDK are not implemented. Do not load downloaded scripts into the application process as a shortcut.

Future manifests should declare scoped operations such as URL resolution, tracker feeds, metadata enrichment or completion notifications. Host APIs must enforce these capabilities, restrict destinations, bound responses and time, redact credentials, and isolate code execution. Unknown permissions require explicit user approval; install/update cannot silently widen grants.
