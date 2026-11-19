# ODM browser companion privacy

This statement describes the original Open Download Manager browser companion version 0.2.0. It sends links to a local ODM application on the same computer.

## Information used and sent

The popup can read the active tab's URL to prefill the link field. It sends a URL only when you choose **Download with ODM** or **Download link with ODM**. The complete selected URL, including its query string or magnet parameters, goes to your paired ODM application. URLs can contain private access links or torrent tracker passkeys; review them before sharing a download or diagnostic report.

The local request includes an extension identity and a pairing token so ODM can authenticate it. The extension does not send download links to an ODM-operated server. ODM then contacts the download source or BitTorrent peers/trackers to perform the requested transfer; those services can observe the requested resource and your network address. This is a local download application, not an anonymous proxy.

Mozilla classifies selected URLs as browsing activity and treats delivery to a native application as transmission outside the browser. The Firefox manifest therefore declares this transmission even though delivery is to your own computer.

## Local storage and controls

The extension stores its paired local endpoint, identity and token in browser extension **local** storage, not sync storage. It does not maintain a browsing-history database or a separate download URL history. The ODM application separately stores transfer state, URLs and limited history locally as described in its documentation.

**Forget pairing** removes the extension's stored connection. **Revoke browser connections** in ODM Settings invalidates the app's pairing tokens. Pairing also expires when the ODM process restarts. Uninstalling the companion removes its browser storage; it does not delete downloads or transfer records held by the separate ODM application. Keep your operating-system account and app data private.

## Permissions and excluded behavior

- `activeTab`: prefill the active page URL when you open the companion.
- `contextMenus`: offer an action for a link you explicitly choose.
- `storage`: keep the connection information on this device.
- `http://127.0.0.1/*`: send the selected link to your paired loopback endpoint.

The companion has no content scripts, cookie extraction, browsing-history permission, automatic download interception, analytics, advertising, remote executable code or account/payment service. It does not automatically forward login cookies or authenticated browser sessions. It does not silently install software.

## Support

Source and issue reports: <https://github.com/abhisekp/open-download-manager>. Do not post private URLs, pairing JSON, launch links, tokens or personal file paths in public issues. Changes to these practices must be reflected in the extension, permissions and listing before release.
