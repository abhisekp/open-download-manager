# Contributing

ODM is an independent MIT-licensed project. Do not copy code or assets from FDM, MDM, or other projects without reviewing the applicable license and preserving required notices. Third-party licenses remain separate from ODM's own license.

Use the Go toolchain pinned in go.mod and Node.js 22 or later. Build the UI before compiling Go because production assets are embedded. Run the README checks, the nested `internal/nettoolscompat` tests, and browser-extension contract tests. Use local fixtures for transfer tests and do not contact public swarms as part of automated tests.

Keep commits signed and independently verifiable. Keep tokens, cookies, signed download URLs, private browser screenshots and user session data out of source and issue reports. PR workflows use read-only repository permissions; publication belongs in separately approved workflows.
