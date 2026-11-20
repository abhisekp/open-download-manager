import React, { useEffect, useState } from "react";

export function HistoryExport({ api }) {
  const [info, setInfo] = useState(null),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false),
    [approved, setApproved] = useState(false),
    [sensitive, setSensitive] = useState(false);
  useEffect(() => {
    let alive = true;
    api("history-export")
      .then((value) => {
        if (alive) setInfo(value);
      })
      .catch(() => {
        if (alive)
          setError(
            "Saved history is unavailable. Current downloads still work.",
          );
      });
    return () => {
      alive = false;
    };
  }, []);
  async function save(event) {
    event.preventDefault();
    if (!approved) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await api("history-export", {
        approved: true,
        includeSensitiveDetails: sensitive,
      });
      setNotice(
        `Saved ${result.records} records to ${result.name} in your download folder.`,
      );
      setApproved(false);
      setSensitive(false);
      setInfo(await api("history-export"));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="history-export" aria-label="Local history export">
      <h3>Export saved history</h3>
      <p>
        Save a local JSON file with recorded transfer names, sizes and statuses.
        No files are uploaded. Source links and full folder paths are left out
        by default.
      </p>
      {info && (
        <>
          <p>
            {info.count} saved records · Destination: {info.directory}
          </p>
          <form onSubmit={save}>
            <label className="permission">
              <input
                type="checkbox"
                checked={sensitive}
                disabled={busy}
                onChange={(e) => {
                  setSensitive(e.target.checked);
                  setApproved(false);
                }}
              />
              Include source links and full file paths
            </label>
            {sensitive && (
              <p>
                These details can contain private link tokens and personal
                folder names. Keep the export private.
              </p>
            )}
            <label className="permission">
              <input
                type="checkbox"
                checked={approved}
                disabled={busy}
                onChange={(e) => setApproved(e.target.checked)}
              />
              Save this history export in the download folder shown above
            </label>
            <button disabled={busy || !approved}>
              {busy ? "Saving history…" : "Save history export"}
            </button>
          </form>
        </>
      )}
      {notice && (
        <p role="status" className="notice">
          {notice}
        </p>
      )}
      {error && (
        <p role="alert" className="alert">
          {error}
        </p>
      )}
    </section>
  );
}
