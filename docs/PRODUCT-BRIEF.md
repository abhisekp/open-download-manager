# ODM product brief

## Intended product

Open Download Manager is an independent, MIT-licensed desktop download manager combining the reviewed IDM, FDM and MDM feature sets with a first-class extension/add-on platform. The user reaffirmed this scope on 2026-11-19. The existing development build is an initial implementation milestone; it does not reduce the intended product to a tracker utility, an aria2 interface, or a browser prototype.

IDM, FDM and MDM are functional references, not sources of code, branding or assets to copy. Native torrent support and the user-approved light/blue layout and navy/cyan icon remain requirements. Packaging and verification must cover the requested Windows and Linux architectures, with browser companion extensions and the intended marketplace/Store distribution routes.

## Core and extension responsibilities

| Product area | Core responsibility | Extension opportunities |
|---|---|---|
| Transfers | Reliable downloading, segmentation, integrity, resume, recovery and resource limits | Additional source resolvers, protocols and provider integrations through controlled host interfaces |
| Torrents | Native torrent/magnet engine, metadata, file controls, peers, trackers and seeding policies | Approved tracker providers, metadata enrichment and torrent-feed adapters |
| Organisation | Queues, scheduling, categories, destinations, history, preferences and notifications | Custom rules, destinations, workflow hooks and completion actions |
| Browser integration | Companion extensions, pairing, explicit download handoff and supported credential flows | Website-specific resolvers and optional browser actions |
| Media and collections | Shared job/collection model, progress, cancellation and safe helper management | Video/audio providers, galleries, playlists and site-specific link extraction |
| Automation | Stable, scoped application commands and events | CLI/MCP adapters, integrations and user-configured workflows |
| Desktop experience | Native shell, tray/lifecycle, accessible controls, themes, updates and uninstall behaviour | Registered commands, menus, settings and bounded UI contributions |

Core reliability and ordinary download management must remain usable without installing third-party add-ons. Browser companion extensions connect browsers to ODM; ODM add-ons extend the application's functionality. Both are required, and their permissions, packaging and release processes are distinct.

## Extension platform acceptance criteria

1. Publish an MIT-licensed SDK, versioned host API, schemas, examples and compatibility policy.
2. Support a full install, permission-review, enable, disable, update, revoke and uninstall lifecycle, including local package installation. A central marketplace is an optional distribution channel, not a prerequisite for extensibility.
3. Provide explicit capability-based APIs for source resolution, collections, metadata, controlled transfer operations, events and UI contributions. Add-ons must not depend on undocumented application internals.
4. Isolate executable add-ons from the core process. Select and validate the execution runtime through a focused architecture/security milestone; arbitrary downloaded scripts must not run with unrestricted host access.
5. Enforce approved network destinations, storage boundaries, scoped credential access, timeouts, cancellation and resource limits. An update cannot silently broaden permissions.
6. Define signing/trust, compatibility checks, failed-update recovery and extension crash containment. One broken extension must not break ordinary downloads.
7. Prove the API with several independently packaged examples, including tracker feeds and a URL/source resolver. The current built-in tracker feed alone does not satisfy the platform requirement.

## Delivery and acceptance

Maintain explicit coverage of the reviewed feature sets in the roadmap. Missing features remain outstanding until implemented and verified; any deliberate exclusions require an explicit product decision. Do not report combined feature parity based on a shell, configured dependency, mock UI, or a passing limited test suite.

Prioritise a reliable core and native desktop shell alongside the extension API/SDK foundation, followed by browser integration and richer queue/torrent workflows, then advanced provider capabilities and distribution. Design extension boundaries early so advanced integrations can use the platform rather than become disconnected hard-coded features. Release acceptance includes real user journeys, failure recovery, installed-platform tests, extension permission/isolation tests and actual distribution verification.

For current implementation evidence and outstanding work, see `ROADMAP.md`, `EXTENSIONS.md`, `BROWSERS.md`, `NATIVE-TORRENTS.md` and `VERIFICATION.md`. This brief defines the target, not a claim that these capabilities already exist.
