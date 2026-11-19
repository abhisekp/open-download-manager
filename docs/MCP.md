# MCP integration boundary

No MCP server is implemented, deployed or connected. The invoked MCP Server Engineering workflow requires hosted HTTPS access, active user-bound API keys and caller isolation; a local RPC endpoint is not equivalent.

For a future optional remote controller, expose `list_transfers` and `get_transfer` first, with bounded results, owner-derived authorization, redacted source URLs and no filesystem paths by default. Add write scopes separately (`downloads:create`, `downloads:control`), with explicit local-device consent, restricted URL destinations, quotas and idempotency keys. Never expose generic shell execution, arbitrary aria2 RPC or unrestricted remote file fetching.

A hosted broker must not proxy directly to the app's loopback port. A paired desktop agent needs an outbound authenticated connection, revocable device identity and a user-visible approval model. API keys need hashed storage, issuance, scopes, revocation and audit. Hosting/account/privacy and cost decisions are unresolved; no service was provisioned.
