import React, { useState } from "react";
export function BrowserConnections({ api }) {
  const [origin, setOrigin] = useState(""),
    [pairing, setPairing] = useState(""),
    [status, setStatus] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <section className="boundary">
      <h3>Browser extensions</h3>
      <p>
        Use the ODM extension in Chrome, Edge, Firefox, Brave, Opera, or Vivaldi
        to send links here. Marketplace listings are not published yet;
        development packages are included in the source release. Safari
        conversion source is provided but untested.
      </p>
      <p>
        Open the extension popup, copy its browser identity, and paste it below.
        Pairing grants link submission only and lasts for this ODM session.
      </p>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            const out = await api("pair", { extensionOrigin: origin.trim() });
            setPairing(JSON.stringify(out, null, 2));
            setStatus(
              "Copy the pairing JSON into this browser extension. Keep it private.",
            );
          } catch (e) {
            setStatus(e.message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <label className="field">
          Browser extension identity
          <input
            required
            value={origin}
            onChange={(e) => setOrigin(e.target.value)}
            placeholder="chrome-extension://… or moz-extension://…"
          />
        </label>
        <button disabled={busy} className="primary">
          Pair browser
        </button>
      </form>
      {pairing && (
        <label className="field">
          Pairing JSON
          <textarea readOnly rows="6" value={pairing} />
        </label>
      )}
      <p role="status">{status}</p>
      <button
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          try {
            await api("revoke-browsers", {});
            setPairing("");
            setStatus("All browser connections revoked.");
          } catch (e) {
            setStatus(e.message);
          } finally {
            setBusy(false);
          }
        }}
      >
        Revoke browser connections
      </button>
    </section>
  );
}
