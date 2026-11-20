import React, { useEffect, useState } from "react";

export function TorrentLimits({ api }) {
  const [loaded, setLoaded] = useState(false),
    [busy, setBusy] = useState(false),
    [down, setDown] = useState(""),
    [up, setUp] = useState(""),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  useEffect(() => {
    let alive = true;
    api("torrent-limits")
      .then((value) => {
        if (alive) {
          setDown(String(value.downloadKiB));
          setUp(String(value.uploadKiB));
          setLoaded(true);
        }
      })
      .catch(() => {
        if (alive)
          setError(
            "Native torrent limits are unavailable. Other downloads still work.",
          );
      });
    return () => {
      alive = false;
    };
  }, []);
  async function save(event) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const saved = await api("torrent-limits", {
        downloadKiB: Number(down),
        uploadKiB: Number(up),
      });
      setDown(String(saved.downloadKiB));
      setUp(String(saved.uploadKiB));
      setNotice("Torrent limits saved and applied to active transfers.");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-label="Native torrent bandwidth" className="torrent-limits">
      <h3>Torrent bandwidth</h3>
      <p>
        Live limits shared by native torrent peers and webseeds. Use 0 for
        unlimited. Values are KiB per second. HTTP/FTP/SFTP limits are separate.
        Torrents still stop on completion.
      </p>
      {loaded && (
        <form onSubmit={save}>
          <label className="field">
            Torrent download limit (KiB/s)
            <input
              type="number"
              min="0"
              max="1048576"
              step="1"
              required
              value={down}
              onChange={(e) => setDown(e.target.value)}
            />
          </label>
          <label className="field">
            Torrent upload limit (KiB/s)
            <input
              type="number"
              min="0"
              max="1048576"
              step="1"
              required
              value={up}
              onChange={(e) => setUp(e.target.value)}
            />
          </label>
          <button disabled={busy}>
            {busy ? "Saving torrent limits…" : "Save torrent limits"}
          </button>
        </form>
      )}
      {error && (
        <p role="alert" className="alert">
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="notice">
          {notice}
        </p>
      )}
    </section>
  );
}
