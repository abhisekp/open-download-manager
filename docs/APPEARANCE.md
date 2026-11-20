# Persistent appearance

Settings → Appearance offers Light, Dark and Follow system. The default remains the approved light layout. All three retain the existing navigation, tables, details and approved icon. Dark appearance changes color tokens rather than replacing the layout. Follow system responds to live OS color-scheme changes.

The choice is stored in `appearance.json` in the selected ODM data profile, through the owner-authenticated `/api/preferences` endpoint. It is restored across browser reloads and full application restarts, including memory-only desktop renderer sessions. No browser account or cloud synchronization is used. Another open browser tab picks up the choice when reloaded.

The API accepts only the three enumerated values, rejects unknown/trailing/oversized input and retains the existing bearer/Host/Origin protections. Writes complete before the active choice changes; failed writes leave the current preference intact. A malformed existing file is retained and only appearance customization becomes unavailable, while downloads remain usable in light mode.

Dark surfaces, text, selected rows, notices, errors, progress tracks, inputs and focus outlines use explicit color tokens. Placeholder text receives a separate dark color. Reduced-motion preferences are respected. Complete keyboard/screen-reader and translated/RTL acceptance remain separate requirements; adding a theme is not a full accessibility certification.

Local tests verify persistence, corrupt-file preservation, failed-write rollback, owner-only edits and invalid input. Real browser checks exercised all three choices, reload restoration, both live system color-scheme changes and restoration in a newly started app process. Actual screenshots are maintained with the user-facing progress artifacts.
