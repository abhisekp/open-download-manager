# Browser integration and distribution

Original extension code is MIT licensed. The extension sends only a link explicitly selected by the user to a paired local ODM instance. No content scripts, browsing-history collection, cookie extraction, arbitrary remote scripts, or automatic download interception are included.

## Prepared targets

| Browser | Source/package route | Current verification |
|---|---|---|
| Chrome | Chromium Manifest V3 / Chrome Web Store | Package and contract tests only |
| Microsoft Edge | Chromium Manifest V3 / Edge Add-ons | Package and contract tests only |
| Brave, Vivaldi | Chromium package / Chrome Web Store | Not browser-tested |
| Opera | Chromium package / Opera Add-ons, subject to review | Not browser-tested |
| Firefox | Firefox MV3 background-script package / Mozilla Add-ons | Firefox 160.0.1 on Windows: installed temporary extension, real popup pairing/transfer/revocation/disconnect/uninstall passed; marketplace signing pending |
| Safari | WebExtension conversion source / Apple distribution | Unconverted, unsigned, untested |

No marketplace listing exists yet. Do not invent extension IDs or store URLs. Installed-browser verification, store artwork/metadata, privacy declarations, policy review and signing/submission remain release gates. Firefox publisher login has been verified. Edge currently redirects to Partner Center Home; access to its extension dashboard is not yet verified. Chrome registration is blocked because the owner's cards cannot pay its registration fee; keep the manual-install package available without purchases or payment changes. Apple Store publication and paid Apple membership are excluded by the owner, so Safari distribution remains unfulfilled.

Firefox packages require version 142 or later and declare `browsingActivity` transmission: user-selected URLs leave the browser for the paired local application. Local-only delivery is still transmission under Mozilla's policy; `none` would be inaccurate. The extension does not upload URLs to ODM-operated servers or maintain a browsing-history database. Mozilla's `web-ext` validator must pass with warnings treated as errors before submission.

See the [browser companion privacy statement](BROWSER-PRIVACY.md). The installed Firefox integration test uses a freshly created synthetic profile, a verified Mozilla browser/driver, its own temporary extension, a local fixture and a separate ODM profile. Its driver enables privileged Firefox UI access only to open that installed extension's internal popup; it never attaches to an existing personal browser or publisher session.

## Pairing and use

1. For development only, load `dist/extension-chromium` with Chromium's unpacked-extension developer workflow, or temporarily load the Firefox package through `about:debugging`. Normal Firefox distribution requires Mozilla signing. Users must explicitly approve installation. Development loading is not a marketplace release.
2. Open the extension popup and expand **Connect to ODM**. Copy its browser identity.
3. In the running ODM application's Settings, paste that identity and choose **Pair browser**.
4. Copy the returned pairing JSON into the extension popup and save it.
5. Paste a download URL or use **Download link with ODM** in a link's context menu. A page URL is downloaded as that resource; it is not automatically converted into video or file links.
6. Revoke connections in ODM Settings to invalidate extension tokens. Forgetting pairing in the extension only deletes its local copy. Pair again after ODM restarts.

The token is stored in the browser's local extension storage, never sync storage. It grants only `downloads:create`; it cannot read history, control existing transfers, change settings, or invoke arbitrary RPC. The bridge binds to loopback, checks paired extension identity, verifies a dedicated token, rejects website origins and applies a per-browser rate limit. No browser cookies are forwarded, so logged-in downloads that need browser credentials are not yet supported.

## Installer behavior

For consumer Chrome on Windows/macOS, silent arbitrary extension installation is not an ordinary supported distribution path. ODM must not change enterprise policies or weaken browser security to force an install. Once approved listings exist, a native installer may offer an explicit **Open extension marketplace** step for the detected browser. The browser/store still owns the installation confirmation. No installer or forced-extension-install registry keys are provided in this milestone.

Safari source can be converted with Apple's tooling, but conversion, real Safari testing and a supported signed distribution route are not implemented. Apple Store publication and paid Apple enrollment are excluded by the owner. A ZIP of source is not an installable Safari extension.

## Build/test

```sh
node --test browser-extension/test.cjs
python scripts/extensions.py
npx --yes web-ext@10.8.0 lint --source-dir dist/extension-firefox --warnings-as-errors
```

A future protected marketplace workflow should upload reviewed immutable extension packages using least-privilege store credentials entered outside source control. Each browser's publication, permission changes and review outcomes must be independently verified. The current GitHub workflows build artifacts only and do not submit to stores.

Official distribution references checked 2026-11-19:
- https://developer.chrome.com/docs/extensions/how-to/distribute
- https://developer.chrome.com/docs/extensions/how-to/distribute/install-extensions
- https://extensionworkshop.com/documentation/publish/signing-and-distribution-overview/
- https://learn.microsoft.com/en-us/microsoft-edge/extensions/publish/publish-extension
- https://developer.apple.com/documentation/safariservices/packaging-and-distributing-safari-web-extensions-with-app-store-connect
