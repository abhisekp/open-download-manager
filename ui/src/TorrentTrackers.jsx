import React, { useEffect, useState } from "react";

export function TorrentTrackers({ gid, api, suggestions }) {
  const [state, setState] = useState(null);
  const [text, setText] = useState("");
  const [approved, setApproved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [mode, setMode] = useState("add");
  const [expected, setExpected] = useState([]);
  async function reload() {
    setBusy(true);
    try {
      const current = await api(
        "torrent-trackers?gid=" + encodeURIComponent(gid),
      );
      setState(current);
      setApproved(false);
      setMode("add");
      setText("");
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    setState(null);
    setApproved(false);
    setMode("add");
    setText("");
    setError("");
    setNotice("");
    let current = true;
    api("torrent-trackers?gid=" + encodeURIComponent(gid))
      .then((value) => {
        if (current) setState(value);
      })
      .catch((failure) => {
        if (current) setError(failure.message);
      });
    return () => {
      current = false;
    };
  }, [gid]);

  async function add(event) {
    event.preventDefault();
    if (!approved || busy) return;
    const urls = text
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean);
    if (mode === "add" && (!urls.length || urls.length > 32)) {
      setError("Choose between 1 and 32 tracker URLs.");
      return;
    }
    if (mode === "replace" && urls.length > 128) {
      setError("Choose at most 128 replacement tracker URLs.");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await api("torrent-trackers", {
        gid,
        urls,
        approved: true,
        action: mode,
        ...(mode === "replace" ? { expected } : {}),
      });
      if (mode === "replace")
        setNotice(
          `Tracker list replaced with ${result.urls.length} URLs. Torrent remains paused.`,
        );
      else {
        setNotice(
          `${result.added.length} trackers added; ${result.skipped.length} duplicates skipped.`,
        );
        if (result.error)
          setError(result.error + ` ${result.remaining} not added.`);
        else setText("");
      }
      await reload();
    } catch (failure) {
      setError(failure.message);
    } finally {
      setApproved(false);
      setBusy(false);
    }
  }

  return (
    <details className="torrent-trackers">
      <summary>Trackers</summary>
      {!state && !error && <p role="status">Loading tracker details…</p>}
      {state && (
        <>
          <button
            type="button"
            disabled={busy}
            onClick={() =>
              reload().catch((failure) => setError(failure.message))
            }
          >
            Refresh trackers
          </button>
          {state.urls.length ? (
            <ul>
              {state.urls.map((url) => (
                <li key={url}>
                  <code>{url}</code>
                </li>
              ))}
            </ul>
          ) : (
            <p>No trackers are available to display.</p>
          )}
          {!state.canAdd && <p>{state.reason}</p>}
          {state.metadataReady && !state.private && !state.canReplace && (
            <p>
              Pause the torrent and wait until it has stopped, then refresh to
              remove or replace trackers.
            </p>
          )}
          {state.canAdd && (
            <form onSubmit={add}>
              <label>
                Tracker operation
                <select
                  value={mode}
                  disabled={busy}
                  onChange={(event) => {
                    const value = event.target.value;
                    setMode(value);
                    setApproved(false);
                    setNotice("");
                    setError("");
                    setExpected([...state.urls]);
                    setText(value === "replace" ? state.urls.join("\n") : "");
                  }}
                >
                  <option value="add">Add trackers</option>
                  {state.canReplace && (
                    <option value="replace">
                      Replace complete tracker list
                    </option>
                  )}
                </select>
              </label>
              <p>
                {mode === "replace"
                  ? "Only the URLs below will remain. A blank list removes every tracker and can prevent peer discovery. Replacement is saved together; the torrent remains paused."
                  : "Existing trackers are kept."}{" "}
                New trackers can receive this torrent’s info hash and your
                network address.
              </p>
              <label>
                {mode === "replace"
                  ? "Complete replacement tracker list, one per line (up to 128)"
                  : "Tracker URLs, one per line (up to 32)"}
                <textarea
                  rows="6"
                  maxLength={mode === "replace" ? 262144 : 65536}
                  value={text}
                  disabled={busy}
                  onChange={(event) => {
                    setText(event.target.value);
                    setApproved(false);
                  }}
                />
              </label>
              {suggestions.length > 0 && (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    setText(suggestions.slice(0, 32).join("\n"));
                    setApproved(false);
                    setNotice(
                      `Copied ${Math.min(suggestions.length, 32)} fetched trackers for review. Nothing has been added yet.`,
                    );
                  }}
                >
                  Review fetched trackers
                </button>
              )}
              <label className="permission">
                <input
                  type="checkbox"
                  checked={approved}
                  disabled={busy}
                  onChange={(event) => setApproved(event.target.checked)}
                />{" "}
                {mode === "replace"
                  ? "Approve this complete tracker replacement, including removed URLs"
                  : "Allow these tracker URLs for this public torrent"}
              </label>
              <button
                type="submit"
                disabled={busy || !approved || (mode === "add" && !text.trim())}
              >
                {busy
                  ? "Saving trackers…"
                  : mode === "replace"
                    ? "Replace reviewed tracker list"
                    : "Add reviewed trackers"}
              </button>
            </form>
          )}
        </>
      )}
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </details>
  );
}
